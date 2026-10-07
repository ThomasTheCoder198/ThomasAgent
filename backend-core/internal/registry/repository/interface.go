package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/model"
)

type ProviderRepository interface {
	InsertProvider(context.Context, postgres.DBTX, model.ProviderInput, *uuid.UUID) (model.Provider, error)
	GetProvider(context.Context, postgres.DBTX, uuid.UUID) (ProviderRow, error)
	GetProviderForUpdate(context.Context, postgres.DBTX, uuid.UUID) (ProviderRow, error)
	UpdateProvider(context.Context, postgres.DBTX, uuid.UUID, string, string, bool, *uuid.UUID) (model.Provider, error)
	DeleteProvider(context.Context, postgres.DBTX, uuid.UUID) (*uuid.UUID, error)
	ListProviders(context.Context, postgres.DBTX) ([]model.Provider, error)
}

type ModelRepository interface {
	UpsertModel(context.Context, postgres.DBTX, model.ModelInput, string) (model.Model, bool, error)
	DeleteModel(context.Context, postgres.DBTX, uuid.UUID) error
	GetModel(context.Context, postgres.DBTX, uuid.UUID) (model.Model, error)
	GetModelForShare(context.Context, postgres.DBTX, uuid.UUID) (model.Model, error)
	ListModels(context.Context, postgres.DBTX, *uuid.UUID, *model.Capability) ([]model.Model, error)
	DeleteStaleSyncModels(context.Context, postgres.DBTX, uuid.UUID, []uuid.UUID) error
}

type RoleRepository interface {
	UpsertRole(context.Context, postgres.DBTX, model.Role, uuid.UUID) error
	ListRoles(context.Context, postgres.DBTX) ([]model.RoleAssignment, error)
	RoleModelID(context.Context, postgres.DBTX, model.Role) (uuid.UUID, bool, error)
}

type RegistryRepository interface {
	ProviderRepository
	ModelRepository
	RoleRepository
}

type PGRepository struct{}

func NewRegistryRepository() *PGRepository { return &PGRepository{} }

var _ RegistryRepository = PGRepository{}
