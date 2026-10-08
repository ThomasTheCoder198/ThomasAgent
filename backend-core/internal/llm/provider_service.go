package llm

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

const (
	auditTargetProvider       = "provider"
	auditActionProviderCreate = "provider.create"
	auditActionProviderUpdate = "provider.update"
	auditActionProviderDelete = "provider.delete"
)

func (s *Service) CreateProvider(ctx context.Context, input ProviderInput) (Provider, error) {
	if err := normalizeProvider(&input); err != nil {
		return Provider{}, err
	}
	var created Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var secretID *uuid.UUID
		if input.APIKey != nil && *input.APIKey != "" {
			id, err := s.secrets.Put(ctx, tx, *input.APIKey)
			if err != nil {
				return err
			}
			secretID = &id
		}
		provider, err := s.repository.InsertProvider(ctx, tx, input, secretID)
		if err != nil {
			return err
		}
		created = provider
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionProviderCreate, TargetType: auditTargetProvider,
			TargetID: provider.ID.String(), Metadata: map[string]any{"kind": string(provider.Kind), "name": provider.Name}})
	})
	return created, err
}

// UpdateProvider merges the input over the stored row; the kind can never change.
func (s *Service) UpdateProvider(ctx context.Context, id uuid.UUID, input ProviderInput) (Provider, error) {
	var updated Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := s.repository.GetProviderForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if input.Kind != "" && input.Kind != current.Kind {
			return errors.FieldError("kind", fieldInvalid)
		}
		merged := ProviderInput{Kind: current.Kind, Name: firstNonEmpty(input.Name, current.Name), BaseURL: firstNonEmpty(input.BaseURL, current.BaseURL)}
		if err := normalizeProvider(&merged); err != nil {
			return err
		}
		enabled := current.Enabled
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		secretID, err := s.nextProviderSecret(ctx, tx, current, merged.BaseURL, input.APIKey)
		if err != nil {
			return err
		}
		provider, err := s.repository.UpdateProvider(ctx, tx, id, merged.Name, merged.BaseURL, enabled, secretID)
		if err != nil {
			return err
		}
		updated = provider
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionProviderUpdate, TargetType: auditTargetProvider,
			TargetID: id.String(), Metadata: map[string]any{"keyChanged": input.APIKey != nil}})
	})
	return updated, err
}

// nextProviderSecret drops the stored key when the base URL changes without a new key,
// so a key is never sent to a host it was not entered for.
func (s *Service) nextProviderSecret(ctx context.Context, tx pgx.Tx, current storedProvider, nextBaseURL string, nextKey *string) (*uuid.UUID, error) {
	if nextKey == nil && nextBaseURL != current.BaseURL && current.SecretID != nil {
		if err := s.secrets.Delete(ctx, tx, *current.SecretID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return s.applyKeyChange(ctx, tx, current.SecretID, nextKey)
}

// applyKeyChange: nil keeps the key, "" deletes it, anything else stores or replaces it.
func (s *Service) applyKeyChange(ctx context.Context, tx pgx.Tx, currentSecretID *uuid.UUID, nextKey *string) (*uuid.UUID, error) {
	switch {
	case nextKey == nil:
		return currentSecretID, nil
	case *nextKey == "" && currentSecretID != nil:
		err := s.secrets.Delete(ctx, tx, *currentSecretID)
		return nil, errors.Normalize(err)
	case *nextKey == "":
		return nil, nil
	case currentSecretID != nil:
		err := s.secrets.Replace(ctx, tx, *currentSecretID, *nextKey)
		return currentSecretID, errors.Normalize(err)
	default:
		id, err := s.secrets.Put(ctx, tx, *nextKey)
		return &id, errors.Normalize(err)
	}
}

func (s *Service) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		secretID, err := s.repository.DeleteProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		if secretID != nil {
			if err := s.secrets.Delete(ctx, tx, *secretID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionProviderDelete, TargetType: auditTargetProvider, TargetID: id.String()})
	})
}

func (s *Service) ListProviders(ctx context.Context) ([]Provider, error) {
	providers, err := s.repository.ListProviders(ctx, s.pool)
	return providers, errors.Normalize(err)
}

// TestProvider lists the provider's remote models to prove the key and base URL work.
func (s *Service) TestProvider(ctx context.Context, id uuid.UUID) (int, error) {
	provider, apiKey, err := s.providerWithKey(ctx, id)
	if err != nil {
		return 0, err
	}
	models, err := s.catalog.ListModels(ctx, provider.ID, provider.Kind, provider.BaseURL, apiKey)
	if err != nil {
		return 0, err
	}
	return len(models), nil
}

func (s *Service) providerWithKey(ctx context.Context, id uuid.UUID) (storedProvider, string, error) {
	provider, err := s.repository.GetProvider(ctx, s.pool, id)
	if err != nil {
		return storedProvider{}, "", errors.Normalize(err)
	}
	if provider.SecretID == nil {
		return provider, "", nil
	}
	apiKey, err := s.secrets.Get(ctx, s.pool, *provider.SecretID)
	return provider, apiKey, errors.Normalize(err)
}

func normalizeProvider(input *ProviderInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if !IsValidProviderKind(input.Kind) {
		return errors.FieldError("kind", fieldInvalid)
	}
	if input.Name == "" {
		return errors.FieldError("name", fieldRequired)
	}
	if input.BaseURL == "" {
		input.BaseURL = DefaultBaseURL(input.Kind)
	}
	if input.BaseURL == "" {
		return errors.FieldError("baseUrl", fieldRequired)
	}
	return validateBaseURL(input.BaseURL)
}

func validateBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || !outbound.IsSupportedScheme(parsed.Scheme) {
		return errors.FieldError("baseUrl", fieldInvalid)
	}
	return nil
}

func firstNonEmpty(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}
