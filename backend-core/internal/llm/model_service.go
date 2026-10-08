package llm

import (
	"context"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
)

const (
	modelSourceManual      = "manual"
	modelSourceSync        = "sync"
	auditTargetModel       = "model"
	auditActionModelCreate = "model.create"
	auditActionModelDelete = "model.delete"
	auditActionModelSync   = "model.sync"
)

func (s *Service) CreateModel(ctx context.Context, input ModelInput) (Model, error) {
	if err := validateModel(&input); err != nil {
		return Model{}, err
	}
	var created Model
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		model, _, err := s.repository.UpsertModel(ctx, tx, input, modelSourceManual)
		if err != nil {
			return err
		}
		created = model
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelCreate, TargetType: auditTargetModel, TargetID: model.ID.String()})
	})
	return created, err
}

func (s *Service) DeleteModel(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repository.DeleteModel(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelDelete, TargetType: auditTargetModel, TargetID: id.String()})
	})
}

func (s *Service) ListModels(ctx context.Context, providerID *uuid.UUID, capability *Capability) ([]Model, error) {
	models, err := s.repository.ListModels(ctx, s.pool, providerID, capability)
	return models, errors.Normalize(err)
}

// SyncModels copies an OpenRouter provider's published catalog into models, capped at maxRemoteModels.
func (s *Service) SyncModels(ctx context.Context, providerID uuid.UUID) (SyncResult, error) {
	provider, apiKey, err := s.providerWithKey(ctx, providerID)
	if err != nil {
		return SyncResult{}, err
	}
	if provider.Kind != KindOpenRouter {
		return SyncResult{}, errors.ErrLLMProviderSyncUnsupported
	}
	remote, err := s.catalog.ListModels(ctx, provider.ID, provider.Kind, provider.BaseURL, apiKey)
	if err != nil {
		return SyncResult{}, err
	}
	var result SyncResult
	if s.maxRemoteModels <= 0 {
		return SyncResult{}, errors.ErrInternalError
	}
	snapshotComplete := len(remote) > 0 && len(remote) <= s.maxRemoteModels
	if len(remote) > s.maxRemoteModels {
		result.Skipped = len(remote) - s.maxRemoteModels
		remote = remote[:s.maxRemoteModels]
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		kept, err := s.upsertRemoteModels(ctx, tx, providerID, remote, &result)
		if err != nil {
			return err
		}
		// A truncated catalog cannot establish that an omitted row is stale.
		if snapshotComplete {
			if err := s.repository.DeleteStaleSyncModels(ctx, tx, providerID, kept); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelSync, TargetType: auditTargetProvider,
			TargetID: providerID.String(), Metadata: map[string]any{"added": result.Added, "updated": result.Updated, "skipped": result.Skipped}})
	})
	return result, err
}

func (s *Service) upsertRemoteModels(ctx context.Context, tx pgx.Tx, providerID uuid.UUID, remote []RemoteModel, result *SyncResult) ([]uuid.UUID, error) {
	kept := make([]uuid.UUID, 0, len(remote))
	for _, remoteModel := range remote {
		input := ModelInput{ProviderID: providerID, ModelRef: remoteModel.ModelRef, DisplayName: remoteModel.DisplayName,
			Capabilities: remoteModel.Capabilities, ContextWindow: remoteModel.ContextWindow,
			InputPricePerMTok: remoteModel.InputPricePerMTok, OutputPricePerMTok: remoteModel.OutputPricePerMTok}
		if err := validateModel(&input); err != nil {
			return nil, errors.ErrLLMProviderRejected.WithCause(err)
		}
		model, inserted, err := s.repository.UpsertModel(ctx, tx, input, modelSourceSync)
		if err != nil {
			// A manual model with the same ref wins; the remote entry is skipped.
			if errors.ToAppError(err).Code == errors.CodeLLMNameTaken {
				result.Skipped++
				continue
			}
			return nil, err
		}
		if inserted {
			result.Added++
		} else {
			result.Updated++
		}
		kept = append(kept, model.ID)
	}
	return kept, nil
}

func validateModel(input *ModelInput) error {
	if err := validateModelNumbers(input); err != nil {
		return err
	}
	input.ModelRef = strings.TrimSpace(input.ModelRef)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.ModelRef == "" {
		return errors.FieldError("modelRef", fieldRequired)
	}
	if input.DisplayName == "" {
		input.DisplayName = input.ModelRef
	}
	for _, capability := range input.Capabilities {
		if !IsValidCapability(capability) {
			return errors.FieldError("capabilities", fieldInvalid)
		}
		if capability == CapabilityEmbedding && (input.EmbeddingDims == nil || *input.EmbeddingDims <= 0) {
			return errors.FieldError("embeddingDims", fieldRequired)
		}
	}
	return nil
}

func validateModelNumbers(input *ModelInput) error {
	for field, value := range map[string]*int{"contextWindow": input.ContextWindow, "embeddingDims": input.EmbeddingDims} {
		if value != nil && *value <= 0 {
			return errors.FieldError(field, fieldInvalid)
		}
	}
	for field, value := range map[string]*float64{"inputPricePerMTok": input.InputPricePerMTok, "outputPricePerMTok": input.OutputPricePerMTok} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return errors.FieldError(field, fieldInvalid)
		}
	}
	return nil
}
