package service

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/client"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

// ModelSyncService copies an OpenRouter or Google provider's published catalog into its models.
type ModelSyncService interface {
	SyncModels(ctx context.Context, providerID uuid.UUID) (model.SyncResult, error)
}

type ModelSyncServiceImpl struct {
	pool            *pgxpool.Pool
	providers       ProviderService
	models          repository.LLMModelRepo
	catalog         client.ModelCatalog
	maxRemoteModels int
}

func NewModelSyncService(pool *pgxpool.Pool, providers ProviderService, models repository.LLMModelRepo, catalog client.ModelCatalog, maxRemoteModels int) ModelSyncService {
	return &ModelSyncServiceImpl{pool: pool, providers: providers, models: models, catalog: catalog, maxRemoteModels: maxRemoteModels}
}

// SyncModels imports at most maxRemoteModels entries and prunes stale synced rows only when the snapshot is complete.
func (s *ModelSyncServiceImpl) SyncModels(ctx context.Context, providerID uuid.UUID) (model.SyncResult, error) {
	provider, apiKey, err := s.providers.GetProviderWithKey(ctx, providerID)
	if err != nil {
		return model.SyncResult{}, err
	}
	if !syncSupported(provider.Kind) {
		return model.SyncResult{}, errors.ErrLLMProviderSyncUnsupported
	}
	remote, err := s.catalog.ListModels(ctx, provider.ID, provider.Kind, provider.BaseURL, apiKey)
	if err != nil {
		return model.SyncResult{}, err
	}
	remote, dropped := forCore(provider.Kind, remote)
	result := model.SyncResult{Skipped: dropped}
	if s.maxRemoteModels <= 0 {
		return model.SyncResult{}, errors.ErrInternalError
	}
	snapshotComplete := len(remote) > 0 && len(remote) <= s.maxRemoteModels
	if len(remote) > s.maxRemoteModels {
		result.Skipped += len(remote) - s.maxRemoteModels
		remote = remote[:s.maxRemoteModels]
	}
	err = postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		kept, err := s.upsertRemoteModels(ctx, tx, providerID, remote, &result)
		if err != nil {
			return err
		}
		// A truncated catalog cannot establish that an omitted row is stale.
		if snapshotComplete {
			if err := s.models.DeleteStaleSyncModels(ctx, tx, providerID, kept); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelSync, TargetType: auditTargetProvider,
			TargetID: providerID.String(), Metadata: map[string]any{"added": result.Added, "updated": result.Updated, "skipped": result.Skipped}})
	})
	return result, err
}

func syncSupported(kind model.ProviderKind) bool {
	return kind == model.KindOpenRouter || kind == model.KindGoogle
}

// forCore trims a provider's list to what core roles can use and reports how many models it dropped (ruling
// F2a3-1). Core roles never use embeddings (ragx owns them, O6) and Google's list carries no dimensions, so a Google
// sync keeps only models that generate content (chat) and never stores an embedding capability from Google.
func forCore(kind model.ProviderKind, remote []model.RemoteModel) ([]model.RemoteModel, int) {
	if kind != model.KindGoogle {
		return remote, 0
	}
	kept := make([]model.RemoteModel, 0, len(remote))
	for _, remoteModel := range remote {
		if !slices.Contains(remoteModel.Capabilities, model.CapabilityChat) {
			continue
		}
		remoteModel.Capabilities = slices.DeleteFunc(slices.Clone(remoteModel.Capabilities), func(capability model.Capability) bool {
			return capability == model.CapabilityEmbedding
		})
		kept = append(kept, remoteModel)
	}
	return kept, len(remote) - len(kept)
}

func (s *ModelSyncServiceImpl) upsertRemoteModels(ctx context.Context, tx pgx.Tx, providerID uuid.UUID, remote []model.RemoteModel, result *model.SyncResult) ([]uuid.UUID, error) {
	kept := make([]uuid.UUID, 0, len(remote))
	for _, remoteModel := range remote {
		input := model.LLMModelInput{ProviderID: providerID, ModelRef: remoteModel.ModelRef, DisplayName: remoteModel.DisplayName,
			Capabilities: remoteModel.Capabilities, ContextWindow: remoteModel.ContextWindow,
			InputPricePerMTok: remoteModel.InputPricePerMTok, OutputPricePerMTok: remoteModel.OutputPricePerMTok}
		if err := validateLLMModel(&input); err != nil {
			return nil, errors.ErrLLMProviderRejected.WithCause(err)
		}
		llmModel, inserted, err := s.models.UpsertModel(ctx, tx, input, model.ModelSourceSync)
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
		kept = append(kept, llmModel.ID)
	}
	return kept, nil
}
