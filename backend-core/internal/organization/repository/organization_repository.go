package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const organizationColumns = `id, name, external_parsing, ragx_status, ragx_error_code, ragx_default_kb_id,
	settings_synced, created_at, updated_at`

// OrganizationRepo stores organizations. Rows are platform data: every method scopes by the tenant in ctx, which
// is 'default' for the operator.
type OrganizationRepo interface {
	InsertOrganization(ctx context.Context, db postgres.DBTX, id string, settings model.Settings) (model.Organization, error)
	GetOrganization(ctx context.Context, db postgres.DBTX, id string) (model.Organization, error)
	GetOrganizationForUpdate(ctx context.Context, db postgres.DBTX, id string) (model.Organization, error)
	ListOrganizations(ctx context.Context, db postgres.DBTX) ([]model.Organization, error)
	UpdateSettings(ctx context.Context, db postgres.DBTX, id string, settings model.Settings) (model.Organization, error)
	MarkSynced(ctx context.Context, db postgres.DBTX, id string, sent model.Settings, defaultKB uuid.UUID) (model.Organization, error)
	MarkSyncFailed(ctx context.Context, db postgres.DBTX, id string, sent model.Settings, code errors.Code) (model.Organization, error)
}

type organizationRepo struct{}

func NewOrganizationRepo() OrganizationRepo { return organizationRepo{} }

func scanOrganization(row pgx.Row) (model.Organization, error) {
	var org model.Organization
	err := row.Scan(&org.ID, &org.Name, &org.ExternalParsing, &org.RagxStatus, &org.RagxErrorCode, &org.DefaultKnowledgeBaseID,
		&org.SettingsSynced, &org.CreatedAt, &org.UpdatedAt)
	switch {
	case stderrors.Is(err, pgx.ErrNoRows):
		return model.Organization{}, errors.ErrNotFound
	case postgres.IsUniqueViolation(err):
		return model.Organization{}, errors.FieldConflict(model.FieldName)
	case err != nil:
		return model.Organization{}, fmt.Errorf("scan organization: %w", err)
	}
	return org, nil
}

func (organizationRepo) InsertOrganization(ctx context.Context, db postgres.DBTX, id string, settings model.Settings) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	return scanOrganization(db.QueryRow(ctx, `INSERT INTO organizations (id, tenant_id, name, external_parsing, ragx_status)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+organizationColumns, id, tenantID, settings.Name, settings.ExternalParsing, model.RagxStatusPending))
}

func (organizationRepo) GetOrganization(ctx context.Context, db postgres.DBTX, id string) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	return scanOrganization(db.QueryRow(ctx, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1 AND tenant_id = $2`, id, tenantID))
}

func (organizationRepo) GetOrganizationForUpdate(ctx context.Context, db postgres.DBTX, id string) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	return scanOrganization(db.QueryRow(ctx, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenantID))
}

func (organizationRepo) ListOrganizations(ctx context.Context, db postgres.DBTX) ([]model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+organizationColumns+` FROM organizations WHERE tenant_id = $1 ORDER BY lower(name), id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	orgs := []model.Organization{}
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	return orgs, nil
}

func (organizationRepo) UpdateSettings(ctx context.Context, db postgres.DBTX, id string, settings model.Settings) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	return scanOrganization(db.QueryRow(ctx, `UPDATE organizations SET name = $3, external_parsing = $4, settings_synced = false,
		updated_at = now() WHERE id = $1 AND tenant_id = $2 RETURNING `+organizationColumns, id, tenantID, settings.Name, settings.ExternalParsing))
}

// MarkSynced records a successful PUT /v1/orgs. ragx now has an org space and a default KB whatever settings were
// sent, so the org is ready; settings_synced is true only if the row still holds what was sent. The error code is
// cleared when the row still holds what was sent, or when the org was not ready before (the provisioning failure is
// no longer true). A ready org whose newer settings failed keeps that failure's code: those settings are still unsynced.
func (organizationRepo) MarkSynced(ctx context.Context, db postgres.DBTX, id string, sent model.Settings, defaultKB uuid.UUID) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	return scanOrganization(db.QueryRow(ctx, `UPDATE organizations SET ragx_status = $6, ragx_default_kb_id = $5,
		settings_synced = (name = $3 AND external_parsing = $4),
		ragx_error_code = CASE WHEN (name = $3 AND external_parsing = $4) OR ragx_status <> $6 THEN '' ELSE ragx_error_code END,
		updated_at = now()
		WHERE id = $1 AND tenant_id = $2 RETURNING `+organizationColumns, id, tenantID, sent.Name, sent.ExternalParsing, defaultKB, model.RagxStatusReady))
}

// MarkSyncFailed applies only while the row still holds the sent settings: a newer edit owns the row otherwise and
// reports its own result. A ready org stays ready (its Knowledge Bases keep working).
func (r organizationRepo) MarkSyncFailed(ctx context.Context, db postgres.DBTX, id string, sent model.Settings, code errors.Code) (model.Organization, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	org, err := scanOrganization(db.QueryRow(ctx, `UPDATE organizations SET
		ragx_status = CASE WHEN ragx_status = $6 THEN $6 ELSE $7 END,
		settings_synced = false, ragx_error_code = $5, updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND name = $3 AND external_parsing = $4
		RETURNING `+organizationColumns, id, tenantID, sent.Name, sent.ExternalParsing, string(code), model.RagxStatusReady, model.RagxStatusFailed))
	if stderrors.Is(err, errors.ErrNotFound) {
		return r.GetOrganization(ctx, db, id)
	}
	return org, err
}
