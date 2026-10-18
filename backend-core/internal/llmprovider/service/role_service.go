package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// RoleService assigns one model per role after checking the model has the role's capabilities.
type RoleService interface {
	AssignRole(ctx context.Context, role model.Role, modelID uuid.UUID) error
	ListRoles(ctx context.Context) ([]model.RoleAssignment, error)
	ClearRole(ctx context.Context, role model.Role) error
}

type RoleServiceImpl struct {
	pool   *pgxpool.Pool
	models repository.LLMModelRepo
	roles  repository.RoleRepo
}

func NewRoleService(pool *pgxpool.Pool, models repository.LLMModelRepo, roles repository.RoleRepo) RoleService {
	return &RoleServiceImpl{pool: pool, models: models, roles: roles}
}

func (s *RoleServiceImpl) AssignRole(ctx context.Context, role model.Role, modelID uuid.UUID) error {
	if model.RequirementsForRole(role) == nil {
		return errors.FieldError("role", errors.FieldCodeInvalid)
	}
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		llmModel, err := s.models.GetModelForShare(ctx, tx, modelID)
		if err != nil {
			return err
		}
		if !llmModel.FitsRole(role) {
			return capabilityMismatch(role)
		}
		if err := s.roles.UpsertRole(ctx, tx, role, modelID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionRoleAssign, TargetType: auditTargetRole,
			TargetID: string(role), Metadata: map[string]any{"modelId": modelID.String()}})
	})
}

func (s *RoleServiceImpl) ListRoles(ctx context.Context) ([]model.RoleAssignment, error) {
	assignments, err := s.roles.ListRoles(ctx, s.pool)
	return assignments, errors.Normalize(err)
}

// ClearRole removes the role's model for the tenant in ctx (the platform on /api/v1/model-roles); idempotent.
func (s *RoleServiceImpl) ClearRole(ctx context.Context, role model.Role) error {
	if model.RequirementsForRole(role) == nil {
		return errors.FieldError("role", errors.FieldCodeInvalid)
	}
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.roles.DeleteRoleIn(ctx, tx, tenantID, role); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionRoleClear, TargetType: auditTargetRole, TargetID: string(role)})
	})
}

func capabilityMismatch(role model.Role) error {
	return errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{"role": string(role), "requires": model.RequirementsForRole(role)})
}
