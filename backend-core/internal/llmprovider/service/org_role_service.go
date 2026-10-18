package service

import (
	"context"
	"slices"

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

// OrgRoleService overrides the platform's role models for the org in ctx (RequireOrg). Only core's chat roles can
// be overridden; ragx owns embedding and rerank (ruling A2-3).
type OrgRoleService interface {
	ListOrgRoles(ctx context.Context) ([]model.OrgRoleView, error)
	AssignOrgRole(ctx context.Context, role model.Role, modelID uuid.UUID) error
	ClearOrgRole(ctx context.Context, role model.Role) error
}

type OrgRoleServiceImpl struct {
	pool   *pgxpool.Pool
	models repository.LLMModelRepo
	roles  repository.RoleRepo
}

func NewOrgRoleService(pool *pgxpool.Pool, models repository.LLMModelRepo, roles repository.RoleRepo) OrgRoleService {
	return &OrgRoleServiceImpl{pool: pool, models: models, roles: roles}
}

func orgRoleTarget(orgID string, role model.Role) string { return orgID + ":" + string(role) }

func checkOrgRole(role model.Role) error {
	if !slices.Contains(model.CoreChatRoles, role) {
		return errors.FieldError("role", errors.FieldCodeInvalid)
	}
	return nil
}

func (s *OrgRoleServiceImpl) ListOrgRoles(ctx context.Context) ([]model.OrgRoleView, error) {
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	own, err := s.roles.ListRolesIn(ctx, s.pool, orgID)
	if err != nil {
		return nil, errors.Normalize(err)
	}
	views := make([]model.OrgRoleView, 0, len(model.CoreChatRoles))
	for _, role := range model.CoreChatRoles {
		view := model.OrgRoleView{Role: role, Source: model.ResolvedSourceNone}
		if index := slices.IndexFunc(own, func(a model.RoleAssignment) bool { return a.Role == role }); index >= 0 {
			id := own[index].ModelID
			view.ModelID = &id
		}
		if err := s.fillEffective(ctx, &view); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// fillEffective uses the resolver's own lookup (findAssignment); an unassigned role is "none", not an error.
func (s *OrgRoleServiceImpl) fillEffective(ctx context.Context, view *model.OrgRoleView) error {
	found, err := findAssignment(ctx, s.pool, s.roles, view.Role)
	if err != nil {
		if errors.CodeOf(err) == errors.CodeLLMRoleNotAssigned {
			return nil
		}
		return errors.Normalize(err)
	}
	view.EffectiveModelID, view.Source = &found.modelID, found.source
	return nil
}

func (s *OrgRoleServiceImpl) AssignOrgRole(ctx context.Context, role model.Role, modelID uuid.UUID) error {
	if err := checkOrgRole(role); err != nil {
		return err
	}
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		llmModel, err := s.models.GetModelForShare(platformScope(ctx), tx, modelID)
		if err != nil {
			return err
		}
		if !llmModel.FitsRole(role) {
			return capabilityMismatch(role)
		}
		if err := s.roles.UpsertRoleIn(ctx, tx, orgID, role, modelID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionOrgRoleAssign, TargetType: auditTargetRole,
			TargetID: orgRoleTarget(orgID, role), Metadata: map[string]any{"modelId": modelID.String()}})
	})
}

func (s *OrgRoleServiceImpl) ClearOrgRole(ctx context.Context, role model.Role) error {
	if err := checkOrgRole(role); err != nil {
		return err
	}
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.roles.DeleteRoleIn(ctx, tx, orgID, role); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionOrgRoleClear, TargetType: auditTargetRole,
			TargetID: orgRoleTarget(orgID, role)})
	})
}
