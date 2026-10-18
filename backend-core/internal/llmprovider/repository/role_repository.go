package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// RoleRepo stores which model serves each role. The ctx methods scope by the tenant in ctx; the …In methods take the
// tenant explicitly, because resolution under an org reads both the org's rows and the platform's (R2).
type RoleRepo interface {
	UpsertRole(ctx context.Context, db postgres.DBTX, role model.Role, modelID uuid.UUID) error
	ListRoles(ctx context.Context, db postgres.DBTX) ([]model.RoleAssignment, error)
	RoleModelID(ctx context.Context, db postgres.DBTX, role model.Role) (uuid.UUID, bool, error)
	UpsertRoleIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role, modelID uuid.UUID) error
	DeleteRoleIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role) error
	ListRolesIn(ctx context.Context, db postgres.DBTX, tenantID string) ([]model.RoleAssignment, error)
	RoleModelIDIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role) (uuid.UUID, bool, error)
	// RolesUsingModel lists the roles any tenant has pinned to the model; a capability edit must keep them fitting.
	RolesUsingModel(ctx context.Context, db postgres.DBTX, modelID uuid.UUID) ([]model.Role, error)
}

type roleRepo struct{}

func NewRoleRepo() RoleRepo { return roleRepo{} }

func (repo roleRepo) UpsertRole(ctx context.Context, db postgres.DBTX, role model.Role, modelID uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	return repo.UpsertRoleIn(ctx, db, tenantID, role, modelID)
}

func (repo roleRepo) ListRoles(ctx context.Context, db postgres.DBTX) ([]model.RoleAssignment, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	return repo.ListRolesIn(ctx, db, tenantID)
}

// RoleModelID reports false, not an error, when the role has no model assigned.
func (repo roleRepo) RoleModelID(ctx context.Context, db postgres.DBTX, role model.Role) (uuid.UUID, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	return repo.RoleModelIDIn(ctx, db, tenantID, role)
}

func (roleRepo) UpsertRoleIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role, modelID uuid.UUID) error {
	_, err := db.Exec(ctx, `INSERT INTO model_roles (tenant_id, role, model_id) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, role) DO UPDATE SET model_id = EXCLUDED.model_id, updated_at = now()`, role, modelID, tenantID)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

// DeleteRoleIn is idempotent: clearing a role that has no row is not an error.
func (roleRepo) DeleteRoleIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role) error {
	if _, err := db.Exec(ctx, `DELETE FROM model_roles WHERE tenant_id = $1 AND role = $2`, tenantID, role); err != nil {
		return fmt.Errorf("clear role: %w", err)
	}
	return nil
}

func (roleRepo) ListRolesIn(ctx context.Context, db postgres.DBTX, tenantID string) ([]model.RoleAssignment, error) {
	rows, err := db.Query(ctx, `SELECT role, model_id FROM model_roles WHERE tenant_id = $1 ORDER BY role`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	assignments := []model.RoleAssignment{}
	for rows.Next() {
		var assignment model.RoleAssignment
		if err := rows.Scan(&assignment.Role, &assignment.ModelID); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}

func (roleRepo) RoleModelIDIn(ctx context.Context, db postgres.DBTX, tenantID string, role model.Role) (uuid.UUID, bool, error) {
	var modelID uuid.UUID
	err := db.QueryRow(ctx, `SELECT model_id FROM model_roles WHERE role = $1 AND tenant_id = $2`, role, tenantID).Scan(&modelID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get role: %w", err)
	}
	return modelID, true, nil
}

func (roleRepo) RolesUsingModel(ctx context.Context, db postgres.DBTX, modelID uuid.UUID) ([]model.Role, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT role FROM model_roles WHERE model_id = $1 ORDER BY role`, modelID)
	if err != nil {
		return nil, fmt.Errorf("roles using model: %w", err)
	}
	defer rows.Close()
	roles := []model.Role{}
	for rows.Next() {
		var role model.Role
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}
