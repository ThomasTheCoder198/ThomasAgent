package llm

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func (Repository) UpsertRole(ctx context.Context, db postgres.DBTX, role Role, modelID uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO model_roles (tenant_id, role, model_id) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, role) DO UPDATE SET model_id = EXCLUDED.model_id, updated_at = now()`, role, modelID, tenantID)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

func (Repository) ListRoles(ctx context.Context, db postgres.DBTX) ([]RoleAssignment, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT role, model_id FROM model_roles WHERE tenant_id = $1 ORDER BY role`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	assignments := []RoleAssignment{}
	for rows.Next() {
		var assignment RoleAssignment
		if err := rows.Scan(&assignment.Role, &assignment.ModelID); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}

// RoleModelID reports false, not an error, when the role has no model assigned.
func (Repository) RoleModelID(ctx context.Context, db postgres.DBTX, role Role) (uuid.UUID, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var modelID uuid.UUID
	err = db.QueryRow(ctx, `SELECT model_id FROM model_roles WHERE role = $1 AND tenant_id = $2`, role, tenantID).Scan(&modelID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get role: %w", err)
	}
	return modelID, true, nil
}
