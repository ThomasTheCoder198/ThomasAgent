package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/model"
)

const providerColumns = `id, kind, name, base_url, api_key_secret_id IS NOT NULL, enabled, created_at, updated_at`

type ProviderRow struct {
	model.Provider
	SecretID *uuid.UUID
}

func ScanProvider(row pgx.Row) (model.Provider, error) {
	var p model.Provider
	err := row.Scan(&p.ID, &p.Kind, &p.Name, &p.BaseURL, &p.HasAPIKey, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Provider{}, errors.ErrRegistryProviderNotFound
	}
	if err != nil {
		return model.Provider{}, fmt.Errorf("scan provider: %w", err)
	}
	return p, nil
}

func (repo PGRepository) InsertProvider(ctx context.Context, db postgres.DBTX, in model.ProviderInput, secretID *uuid.UUID) (model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Provider{}, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	p, err := ScanProvider(db.QueryRow(ctx, `INSERT INTO providers (tenant_id, kind, name, base_url, api_key_secret_id, enabled)
		VALUES ($6, $1, $2, $3, $4, $5) RETURNING `+providerColumns, in.Kind, in.Name, in.BaseURL, secretID, enabled, tenantID))
	if postgres.IsUniqueViolation(err) {
		return model.Provider{}, errors.ErrRegistryNameTaken
	}
	return p, err
}

func (repo PGRepository) GetProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (ProviderRow, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return ProviderRow{}, err
	}
	var r ProviderRow
	err = db.QueryRow(ctx, `SELECT `+providerColumns+`, api_key_secret_id FROM providers WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&r.ID, &r.Kind, &r.Name, &r.BaseURL, &r.HasAPIKey, &r.Enabled, &r.CreatedAt, &r.UpdatedAt, &r.SecretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return ProviderRow{}, errors.ErrRegistryProviderNotFound
	}
	if err != nil {
		return ProviderRow{}, fmt.Errorf("get provider: %w", err)
	}
	return r, nil
}

func (repo PGRepository) UpdateProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID, name, baseURL string, enabled bool, secretID *uuid.UUID) (model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Provider{}, err
	}
	p, err := ScanProvider(db.QueryRow(ctx, `UPDATE providers SET name = $2, base_url = $3, enabled = $4,
		api_key_secret_id = $5, updated_at = now() WHERE id = $1 AND tenant_id = $6 RETURNING `+providerColumns, id, name, baseURL, enabled, secretID, tenantID))
	if postgres.IsUniqueViolation(err) {
		return model.Provider{}, errors.ErrRegistryNameTaken
	}
	return p, err
}

func (repo PGRepository) DeleteProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (*uuid.UUID, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var secretID *uuid.UUID
	err = db.QueryRow(ctx, `DELETE FROM providers WHERE id = $1 AND tenant_id = $2 RETURNING api_key_secret_id`, id, tenantID).Scan(&secretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return nil, errors.ErrRegistryProviderNotFound
	}
	if postgres.IsForeignKeyViolation(err) {
		return nil, errors.ErrRegistryModelInUse
	}
	if err != nil {
		return nil, fmt.Errorf("delete provider: %w", err)
	}
	return secretID, nil
}

func (repo PGRepository) ListProviders(ctx context.Context, db postgres.DBTX) ([]model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+providerColumns+` FROM providers WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	out := []model.Provider{}
	for rows.Next() {
		p, err := ScanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const modelColumns = `id, provider_id, model_ref, display_name, capabilities, context_window, embedding_dims,
	input_price_per_mtok::float8, output_price_per_mtok::float8, source`

func ScanModel(row pgx.Row) (model.Model, error) {
	var m model.Model
	var caps []string
	err := row.Scan(&m.ID, &m.ProviderID, &m.ModelRef, &m.DisplayName, &caps, &m.ContextWindow, &m.EmbeddingDims,
		&m.InputPricePerMTok, &m.OutputPricePerMTok, &m.Source)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Model{}, errors.ErrRegistryModelNotFound
	}
	if err != nil {
		return model.Model{}, fmt.Errorf("scan model: %w", err)
	}
	m.Capabilities = make([]model.Capability, len(caps))
	for i, c := range caps {
		m.Capabilities[i] = model.Capability(c)
	}
	return m, nil
}

func CapStrings(caps []model.Capability) []string {
	out := make([]string, len(caps))
	for i, c := range caps {
		out[i] = string(c)
	}
	return out
}

// UpsertModel reports whether the row was inserted (true) or updated (false); xmax = 0 marks a fresh insert.
func (repo PGRepository) UpsertModel(ctx context.Context, db postgres.DBTX, in model.ModelInput, source string) (model.Model, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Model{}, false, err
	}
	if _, err := repo.GetProvider(ctx, db, in.ProviderID); err != nil {
		return model.Model{}, false, err
	}
	var inserted bool
	row := db.QueryRow(ctx, `INSERT INTO models (tenant_id, provider_id, model_ref, display_name, capabilities, context_window,
		embedding_dims, input_price_per_mtok, output_price_per_mtok, source)
		VALUES ($10,$1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (provider_id, model_ref) DO UPDATE SET display_name = EXCLUDED.display_name,
			capabilities = EXCLUDED.capabilities, context_window = EXCLUDED.context_window,
			embedding_dims = EXCLUDED.embedding_dims, input_price_per_mtok = EXCLUDED.input_price_per_mtok,
			output_price_per_mtok = EXCLUDED.output_price_per_mtok, updated_at = now()
		WHERE models.source = 'sync' AND EXCLUDED.source = 'sync' AND models.tenant_id = EXCLUDED.tenant_id
		RETURNING `+modelColumns+`, (xmax = 0)`,
		in.ProviderID, in.ModelRef, in.DisplayName, CapStrings(in.Capabilities), in.ContextWindow, in.EmbeddingDims,
		in.InputPricePerMTok, in.OutputPricePerMTok, source, tenantID)
	var m model.Model
	var caps []string
	err = row.Scan(&m.ID, &m.ProviderID, &m.ModelRef, &m.DisplayName, &caps, &m.ContextWindow, &m.EmbeddingDims,
		&m.InputPricePerMTok, &m.OutputPricePerMTok, &m.Source, &inserted)
	if postgres.IsForeignKeyViolation(err) {
		return model.Model{}, false, errors.ErrRegistryProviderNotFound
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Model{}, false, errors.ErrRegistryNameTaken
	}
	if err != nil {
		return model.Model{}, false, fmt.Errorf("upsert model: %w", err)
	}
	m.Capabilities = make([]model.Capability, len(caps))
	for i, c := range caps {
		m.Capabilities[i] = model.Capability(c)
	}
	return m, inserted, nil
}

func (repo PGRepository) DeleteStaleSyncModels(ctx context.Context, db postgres.DBTX, providerID uuid.UUID, keep []uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `DELETE FROM models m WHERE m.tenant_id = $1 AND m.provider_id = $2 AND m.source = 'sync'
		AND NOT (m.id = ANY($3::uuid[])) AND NOT EXISTS (
			SELECT 1 FROM model_roles mr WHERE mr.model_id = m.id
		)`, tenantID, providerID, keep)
	if err != nil {
		return fmt.Errorf("delete stale synced models: %w", err)
	}
	return nil
}

func (repo PGRepository) GetModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Model, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Model{}, err
	}
	return ScanModel(db.QueryRow(ctx, `SELECT `+modelColumns+` FROM models WHERE id = $1 AND tenant_id = $2`, id, tenantID))
}

func (repo PGRepository) ListModels(ctx context.Context, db postgres.DBTX, providerID *uuid.UUID, capability *model.Capability) ([]model.Model, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var capFilter *string
	if capability != nil {
		c := string(*capability)
		capFilter = &c
	}
	rows, err := db.Query(ctx, `SELECT `+modelColumns+` FROM models
		WHERE tenant_id = $3 AND ($1::uuid IS NULL OR provider_id = $1) AND ($2::text IS NULL OR $2 = ANY(capabilities))
		ORDER BY display_name`, providerID, capFilter, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()
	out := []model.Model{}
	for rows.Next() {
		m, err := ScanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (repo PGRepository) DeleteModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `DELETE FROM models WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if postgres.IsForeignKeyViolation(err) {
		return errors.ErrRegistryModelInUse
	}
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrRegistryModelNotFound
	}
	return nil
}

func (repo PGRepository) UpsertRole(ctx context.Context, db postgres.DBTX, role model.Role, modelID uuid.UUID) error {
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

func (repo PGRepository) ListRoles(ctx context.Context, db postgres.DBTX) ([]model.RoleAssignment, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT role, model_id FROM model_roles WHERE tenant_id = $1 ORDER BY role`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	out := []model.RoleAssignment{}
	for rows.Next() {
		var a model.RoleAssignment
		if err := rows.Scan(&a.Role, &a.ModelID); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (repo PGRepository) RoleModelID(ctx context.Context, db postgres.DBTX, role model.Role) (uuid.UUID, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = db.QueryRow(ctx, `SELECT model_id FROM model_roles WHERE role = $1 AND tenant_id = $2`, role, tenantID).Scan(&id)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get role: %w", err)
	}
	return id, true, nil
}

func (repo PGRepository) GetProviderForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (ProviderRow, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return ProviderRow{}, err
	}
	if _, err = db.Exec(ctx, `SELECT id FROM providers WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, tenantID); err != nil {
		return ProviderRow{}, fmt.Errorf("lock provider: %w", err)
	}
	return repo.GetProvider(ctx, db, id)
}
func (repo PGRepository) GetModelForShare(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Model, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Model{}, err
	}
	if _, err = db.Exec(ctx, `SELECT id FROM models WHERE id=$1 AND tenant_id=$2 FOR SHARE`, id, tenantID); err != nil {
		return model.Model{}, fmt.Errorf("lock model: %w", err)
	}
	return repo.GetModel(ctx, db, id)
}
