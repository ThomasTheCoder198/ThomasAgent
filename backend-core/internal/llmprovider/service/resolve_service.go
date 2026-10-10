package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
)

// ResolveService resolves a role to its model and decrypted key; the M2 agent runtime uses it.
type ResolveService interface {
	Resolve(ctx context.Context, role model.Role) (model.ResolvedModel, error)
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

func (s *ResolveServiceImpl) Resolve(ctx context.Context, role model.Role) (model.ResolvedModel, error) {
	if model.RequirementsForRole(role) == nil {
		return model.ResolvedModel{}, errors.FieldError("role", errors.FieldCodeInvalid)
	}
	effectiveRole, fallbackFrom, modelID, err := s.resolveAssignment(ctx, role)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	llmModel, err := s.models.GetModel(ctx, s.pool, modelID)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	provider, apiKey, err := s.providers.GetProviderWithKey(ctx, llmModel.ProviderID)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	if !provider.Enabled {
		return model.ResolvedModel{}, errors.ErrProviderUnavailable
	}
	if !llmModel.FitsRole(effectiveRole) {
		return model.ResolvedModel{}, capabilityMismatch(effectiveRole)
	}
	return model.ResolvedModel{Role: effectiveRole, FallbackFrom: fallbackFrom, ProviderKind: provider.Kind, BaseURL: provider.BaseURL, APIKey: apiKey,
		ModelRef: llmModel.ModelRef, Capabilities: llmModel.Capabilities, EmbeddingDims: llmModel.EmbeddingDims}, nil
}

// resolveAssignment falls back from an unassigned decision role to chat.fast.
func (s *ResolveServiceImpl) resolveAssignment(ctx context.Context, role model.Role) (effectiveRole, fallbackFrom model.Role, modelID uuid.UUID, err error) {
	effectiveRole = role
	modelID, assigned, err := s.roles.RoleModelID(ctx, s.pool, role)
	if err != nil {
		return effectiveRole, fallbackFrom, modelID, err
	}
	if !assigned && role == model.RoleDecision {
		effectiveRole, fallbackFrom = model.RoleChatFast, model.RoleDecision
		modelID, assigned, err = s.roles.RoleModelID(ctx, s.pool, model.RoleChatFast)
		if err != nil {
			return effectiveRole, fallbackFrom, modelID, err
		}
	}
	if !assigned {
		return effectiveRole, fallbackFrom, modelID, errors.ErrLLMRoleNotAssigned.WithDetails(map[string]any{"role": string(role)})
	}
	return effectiveRole, fallbackFrom, modelID, nil
}
