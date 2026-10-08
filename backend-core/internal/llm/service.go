package llm

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

const (
	auditActorOwner = "owner"
	fieldRequired   = errors.FieldCodeRequired
	fieldInvalid    = errors.FieldCodeInvalid
)

// ModelCatalog lists the models a provider publishes; HTTPModelCatalog is the real one.
type ModelCatalog interface {
	ListModels(ctx context.Context, providerID uuid.UUID, kind ProviderKind, baseURL, apiKey string) ([]RemoteModel, error)
}

// Service manages providers, their models and role assignments, and resolves a role to a callable model.
// Methods are split by subject across provider_service.go, model_service.go and role_service.go.
type Service struct {
	pool            *pgxpool.Pool
	secrets         *vault.Store
	catalog         ModelCatalog
	maxRemoteModels int
	repository      Repository
}

func NewService(pool *pgxpool.Pool, secrets *vault.Store, catalog ModelCatalog, maxRemoteModels int) *Service {
	return &Service{pool: pool, secrets: secrets, catalog: catalog, maxRemoteModels: maxRemoteModels, repository: NewRepository()}
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return errors.Normalize(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}
