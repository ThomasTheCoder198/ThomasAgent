package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// ResolveService resolves a role (or an Agent's pinned model) to its model and decrypted key. Under an org context
// it reads the org's role row first, then the platform's (R2); providers and models are always platform rows.
type ResolveService interface {
	Resolve(ctx context.Context, role model.Role) (model.ResolvedModel, error)
	ResolveModel(ctx context.Context, modelID uuid.UUID) (model.ResolvedModel, error)
}

type ResolveServiceImpl struct {
	pool      *pgxpool.Pool
	providers ProviderService
	models    repository.LLMModelRepo
	roles     repository.RoleRepo
}

func NewResolveService(pool *pgxpool.Pool, providers ProviderService, models repository.LLMModelRepo, roles repository.RoleRepo) ResolveService {
	return &ResolveServiceImpl{pool: pool, providers: providers, models: models, roles: roles}
}

// platformScope is the one place resolution reads platform rows under an org request (ruling A2-1).
func platformScope(ctx context.Context) context.Context {
	return tenant.WithID(ctx, tenant.PlatformID)
}

type assignment struct {
	modelID      uuid.UUID
	role         model.Role
	fallbackFrom model.Role
	source       string
}

func (s *ResolveServiceImpl) Resolve(ctx context.Context, role model.Role) (model.ResolvedModel, error) {
	if model.RequirementsForRole(role) == nil {
		return model.ResolvedModel{}, errors.FieldError("role", errors.FieldCodeInvalid)
	}
	found, err := findAssignment(ctx, s.pool, s.roles, role)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	resolved, err := s.load(ctx, found.modelID, found.role)
	if err != nil {
		return model.ResolvedModel{}, err
	}
	resolved.FallbackFrom, resolved.Source = found.fallbackFrom, found.source
	return resolved, nil
}

// ResolveModel loads one platform model for an Agent override; the caller has already checked the role fit.
func (s *ResolveServiceImpl) ResolveModel(ctx context.Context, modelID uuid.UUID) (model.ResolvedModel, error) {
	resolved, err := s.load(ctx, modelID, "")
	if err != nil {
		return model.ResolvedModel{}, err
	}
	resolved.Source = model.ResolvedSourceOrg
	return resolved, nil
}

// load reads the model and its provider as platform rows; role "" skips the capability check.
func (s *ResolveServiceImpl) load(ctx context.Context, modelID uuid.UUID, role model.Role) (model.ResolvedModel, error) {
	platform := platformScope(ctx)
	llmModel, err := s.models.GetModel(platform, s.pool, modelID)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	access, err := s.providers.GetProviderAccess(platform, llmModel.ProviderID)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	if !access.Enabled {
		return model.ResolvedModel{}, errors.ErrProviderUnavailable
	}
	if role != "" && !llmModel.FitsRole(role) {
		return model.ResolvedModel{}, capabilityMismatch(role)
	}
	return model.ResolvedModel{Role: role, ProviderID: access.ID, ProviderKind: access.Kind, BaseURL: access.BaseURL,
		APIKey: access.APIKey, KeyVersion: access.KeyVersion, ModelRef: llmModel.ModelRef, Capabilities: llmModel.Capabilities,
		ContextWindow: llmModel.ContextWindow, EmbeddingDims: llmModel.EmbeddingDims}, nil
}

// findAssignment reads org then platform for role, and for an unassigned decision role does the same for chat.fast.
// OrgRoleService uses it too, so the Settings list and a run can never disagree.
func findAssignment(ctx context.Context, db postgres.DBTX, roles repository.RoleRepo, role model.Role) (assignment, error) {
	found, ok, err := lookupAssignment(ctx, db, roles, role)
	if err != nil || ok {
		return found, err
	}
	if role == model.RoleDecision {
		fallback, ok, err := lookupAssignment(ctx, db, roles, model.RoleChatFast)
		if err != nil {
			return assignment{}, err
		}
		if ok {
			fallback.fallbackFrom, fallback.source = model.RoleDecision, model.ResolvedSourceFallback
			return fallback, nil
		}
	}
	return assignment{}, errors.ErrLLMRoleNotAssigned.WithDetails(map[string]any{"role": string(role)})
}

func lookupAssignment(ctx context.Context, db postgres.DBTX, roles repository.RoleRepo, role model.Role) (assignment, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return assignment{}, false, err
	}
	if tenantID != tenant.PlatformID {
		modelID, ok, err := roles.RoleModelIDIn(ctx, db, tenantID, role)
		if err != nil || ok {
			return assignment{modelID: modelID, role: role, source: model.ResolvedSourceOrg}, ok, err
		}
	}
	modelID, ok, err := roles.RoleModelIDIn(ctx, db, tenant.PlatformID, role)
	return assignment{modelID: modelID, role: role, source: model.ResolvedSourcePlatform}, ok, err
}
