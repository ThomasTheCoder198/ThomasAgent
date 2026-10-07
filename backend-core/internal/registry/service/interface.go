package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/model"
)

type Catalog interface {
	ListModels(context.Context, uuid.UUID, model.ProviderKind, string, string) ([]model.RemoteModel, error)
}

// Resolver is the behavior required by the internal model-resolution route.
type Resolver interface {
	Resolve(context.Context, model.Role) (model.ResolvedModel, error)
}

// RegistryService is the behavior required by the public registry HTTP handler.
type RegistryService interface {
	CreateProvider(context.Context, model.ProviderInput) (model.Provider, error)
	UpdateProvider(context.Context, uuid.UUID, model.ProviderInput) (model.Provider, error)
	DeleteProvider(context.Context, uuid.UUID) error
	ListProviders(context.Context) ([]model.Provider, error)
	TestProvider(context.Context, uuid.UUID) (int, error)
	SyncModels(context.Context, uuid.UUID) (model.SyncResult, error)
	CreateModel(context.Context, model.ModelInput) (model.Model, error)
	DeleteModel(context.Context, uuid.UUID) error
	ListModels(context.Context, *uuid.UUID, *model.Capability) ([]model.Model, error)
	AssignRole(context.Context, model.Role, uuid.UUID) error
	ListRoles(context.Context) ([]model.RoleAssignment, error)
}
