package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/client"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

type ProviderService interface {
	CreateProvider(ctx context.Context, input model.ProviderInput) (model.Provider, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, input model.ProviderInput) (model.Provider, error)
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	ListProviders(ctx context.Context) ([]model.Provider, error)
	// TestProvider lists the provider's remote models to prove the key and base URL work.
	TestProvider(ctx context.Context, id uuid.UUID) (int, error)
	// GetProviderWithKey returns the provider and its decrypted API key, for sync and resolve only.
	GetProviderWithKey(ctx context.Context, id uuid.UUID) (model.StoredProvider, string, error)
}

type ProviderServiceImpl struct {
	pool      *pgxpool.Pool
	secrets   *vault.Store
	providers repository.ProviderRepo
	catalog   client.ModelCatalog
}

func NewProviderService(pool *pgxpool.Pool, secrets *vault.Store, providers repository.ProviderRepo, catalog client.ModelCatalog) ProviderService {
	return &ProviderServiceImpl{pool: pool, secrets: secrets, providers: providers, catalog: catalog}
}

func (s *ProviderServiceImpl) CreateProvider(ctx context.Context, input model.ProviderInput) (model.Provider, error) {
	if err := normalizeProvider(&input); err != nil {
		return model.Provider{}, err
	}
	var created model.Provider
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		var secretID *uuid.UUID
		if input.APIKey != nil && *input.APIKey != "" {
			id, err := s.secrets.Put(ctx, tx, *input.APIKey)
			if err != nil {
				return err
			}
			secretID = &id
		}
		provider, err := s.providers.InsertProvider(ctx, tx, input, secretID)
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
func (s *ProviderServiceImpl) UpdateProvider(ctx context.Context, id uuid.UUID, input model.ProviderInput) (model.Provider, error) {
	var updated model.Provider
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := s.providers.GetProviderForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if input.Kind != "" && input.Kind != current.Kind {
			return errors.FieldError("kind", errors.FieldCodeInvalid)
		}
		merged := model.ProviderInput{Kind: current.Kind, Name: firstNonEmpty(input.Name, current.Name), BaseURL: firstNonEmpty(input.BaseURL, current.BaseURL)}
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
		provider, err := s.providers.UpdateProvider(ctx, tx, id, merged.Name, merged.BaseURL, enabled, secretID)
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
func (s *ProviderServiceImpl) nextProviderSecret(ctx context.Context, tx pgx.Tx, current model.StoredProvider, nextBaseURL string, nextKey *string) (*uuid.UUID, error) {
	if nextKey == nil && nextBaseURL != current.BaseURL && current.SecretID != nil {
		if err := s.secrets.Delete(ctx, tx, *current.SecretID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return s.applyKeyChange(ctx, tx, current.SecretID, nextKey)
}

// applyKeyChange: nil keeps the key, "" deletes it, anything else stores or replaces it.
func (s *ProviderServiceImpl) applyKeyChange(ctx context.Context, tx pgx.Tx, currentSecretID *uuid.UUID, nextKey *string) (*uuid.UUID, error) {
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

func (s *ProviderServiceImpl) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		secretID, err := s.providers.DeleteProvider(ctx, tx, id)
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

func (s *ProviderServiceImpl) ListProviders(ctx context.Context) ([]model.Provider, error) {
	providers, err := s.providers.ListProviders(ctx, s.pool)
	return providers, errors.Normalize(err)
}

func (s *ProviderServiceImpl) TestProvider(ctx context.Context, id uuid.UUID) (int, error) {
	provider, apiKey, err := s.GetProviderWithKey(ctx, id)
	if err != nil {
		return 0, err
	}
	remote, err := s.catalog.ListModels(ctx, provider.ID, provider.Kind, provider.BaseURL, apiKey)
	if err != nil {
		return 0, err
	}
	return len(remote), nil
}

func (s *ProviderServiceImpl) GetProviderWithKey(ctx context.Context, id uuid.UUID) (model.StoredProvider, string, error) {
	provider, err := s.providers.GetProvider(ctx, s.pool, id)
	if err != nil {
		return model.StoredProvider{}, "", errors.Normalize(err)
	}
	if provider.SecretID == nil {
		return provider, "", nil
	}
	apiKey, err := s.secrets.Get(ctx, s.pool, *provider.SecretID)
	return provider, apiKey, errors.Normalize(err)
}

func normalizeProvider(input *model.ProviderInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if !model.IsValidProviderKind(input.Kind) {
		return errors.FieldError("kind", errors.FieldCodeInvalid)
	}
	if input.Name == "" {
		return errors.FieldError("name", errors.FieldCodeRequired)
	}
	if input.BaseURL == "" {
		input.BaseURL = model.DefaultBaseURL(input.Kind)
	}
	if input.BaseURL == "" {
		return errors.FieldError("baseUrl", errors.FieldCodeRequired)
	}
	return validateBaseURL(input.BaseURL)
}

func validateBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || !outbound.IsSupportedScheme(parsed.Scheme) {
		return errors.FieldError("baseUrl", errors.FieldCodeInvalid)
	}
	return nil
}

func firstNonEmpty(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}
