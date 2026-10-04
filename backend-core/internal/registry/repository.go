package registry

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

const platformTenantID = "default"

const providerColumns = `id, kind, name, base_url, api_key_secret_id IS NOT NULL, enabled, created_at, updated_at`

type providerRow struct {
	Provider
	secretID *uuid.UUID
}

func scanProvider(row pgx.Row) (Provider, error) {
	var p Provider
	err := row.Scan(&p.ID, &p.Kind, &p.Name, &p.BaseURL, &p.HasAPIKey, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return Provider{}, errProviderNotFound()
	}
	if err != nil {
		return Provider{}, fmt.Errorf("scan provider: %w", err)
	}
	return p, nil
}

func insertProvider(ctx context.Context, db postgres.DBTX, in ProviderInput, secretID *uuid.UUID) (Provider, error) {
	enabled := in.Enabled == nil || *in.Enabled
	p, err := scanProvider(db.QueryRow(ctx, `INSERT INTO providers (tenant_id, kind, name, base_url, api_key_secret_id, enabled)
		VALUES ($6, $1, $2, $3, $4, $5) RETURNING `+providerColumns, in.Kind, in.Name, in.BaseURL, secretID, enabled, platformTenantID))
	if postgres.IsUniqueViolation(err) {
		return Provider{}, errNameTaken()
	}
	return p, err
}

func getProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (providerRow, error) {
	var r providerRow
	err := db.QueryRow(ctx, `SELECT `+providerColumns+`, api_key_secret_id FROM providers WHERE id = $1 AND tenant_id = $2`, id, platformTenantID).
		Scan(&r.ID, &r.Kind, &r.Name, &r.BaseURL, &r.HasAPIKey, &r.Enabled, &r.CreatedAt, &r.UpdatedAt, &r.secretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return providerRow{}, errProviderNotFound()
	}
	if err != nil {
		return providerRow{}, fmt.Errorf("get provider: %w", err)
	}
	return r, nil
}

func updateProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID, name, baseURL string, enabled bool, secretID *uuid.UUID) (Provider, error) {
	p, err := scanProvider(db.QueryRow(ctx, `UPDATE providers SET name = $2, base_url = $3, enabled = $4,
		api_key_secret_id = $5, updated_at = now() WHERE id = $1 AND tenant_id = $6 RETURNING `+providerColumns, id, name, baseURL, enabled, secretID, platformTenantID))
	if postgres.IsUniqueViolation(err) {
		return Provider{}, errNameTaken()
	}
	return p, err
}

func deleteProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (*uuid.UUID, error) {
	var secretID *uuid.UUID
	err := db.QueryRow(ctx, `DELETE FROM providers WHERE id = $1 AND tenant_id = $2 RETURNING api_key_secret_id`, id, platformTenantID).Scan(&secretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return nil, errProviderNotFound()
	}
	if postgres.IsForeignKeyViolation(err) {
		return nil, errModelInUse()
	}
	if err != nil {
		return nil, fmt.Errorf("delete provider: %w", err)
	}
	return secretID, nil
}

func listProviders(ctx context.Context, db postgres.DBTX) ([]Provider, error) {
	rows, err := db.Query(ctx, `SELECT `+providerColumns+` FROM providers WHERE tenant_id = $1 ORDER BY name`, platformTenantID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const modelColumns = `id, provider_id, model_ref, display_name, capabilities, context_window, embedding_dims,
	input_price_per_mtok::float8, output_price_per_mtok::float8, source`

func scanModel(row pgx.Row) (Model, error) {
	var m Model
	var caps []string
	err := row.Scan(&m.ID, &m.ProviderID, &m.ModelRef, &m.DisplayName, &caps, &m.ContextWindow, &m.EmbeddingDims,
		&m.InputPricePerMTok, &m.OutputPricePerMTok, &m.Source)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return Model{}, errModelNotFound()
	}
	if err != nil {
		return Model{}, fmt.Errorf("scan model: %w", err)
	}
	m.Capabilities = make([]Capability, len(caps))
	for i, c := range caps {
		m.Capabilities[i] = Capability(c)
	}
	return m, nil
}

func capStrings(caps []Capability) []string {
	out := make([]string, len(caps))
	for i, c := range caps {
		out[i] = string(c)
	}
	return out
}

// upsertModel reports whether the row was inserted (true) or updated (false); xmax = 0 marks a fresh insert.
func upsertModel(ctx context.Context, db postgres.DBTX, in ModelInput, source string) (Model, bool, error) {
	if _, err := getProvider(ctx, db, in.ProviderID); err != nil {
		return Model{}, false, err
	}
	var inserted bool
	row := db.QueryRow(ctx, `INSERT INTO models (tenant_id, provider_id, model_ref, display_name, capabilities, context_window,
		embedding_dims, input_price_per_mtok, output_price_per_mtok, source)
		VALUES ($10,$1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (provider_id, model_ref) DO UPDATE SET display_name = EXCLUDED.display_name,
			capabilities = EXCLUDED.capabilities, context_window = EXCLUDED.context_window,
			embedding_dims = EXCLUDED.embedding_dims, input_price_per_mtok = EXCLUDED.input_price_per_mtok,
			output_price_per_mtok = EXCLUDED.output_price_per_mtok, updated_at = now()
		RETURNING `+modelColumns+`, (xmax = 0)`,
		in.ProviderID, in.ModelRef, in.DisplayName, capStrings(in.Capabilities), in.ContextWindow, in.EmbeddingDims,
		in.InputPricePerMTok, in.OutputPricePerMTok, source, platformTenantID)
	var m Model
	var caps []string
	err := row.Scan(&m.ID, &m.ProviderID, &m.ModelRef, &m.DisplayName, &caps, &m.ContextWindow, &m.EmbeddingDims,
		&m.InputPricePerMTok, &m.OutputPricePerMTok, &m.Source, &inserted)
	if postgres.IsForeignKeyViolation(err) {
		return Model{}, false, errProviderNotFound()
	}
	if err != nil {
		return Model{}, false, fmt.Errorf("upsert model: %w", err)
	}
	m.Capabilities = make([]Capability, len(caps))
	for i, c := range caps {
		m.Capabilities[i] = Capability(c)
	}
	return m, inserted, nil
}

func getModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) (Model, error) {
	return scanModel(db.QueryRow(ctx, `SELECT `+modelColumns+` FROM models WHERE id = $1 AND tenant_id = $2`, id, platformTenantID))
}

func listModels(ctx context.Context, db postgres.DBTX, providerID *uuid.UUID, capability *Capability) ([]Model, error) {
	var capFilter *string
	if capability != nil {
		c := string(*capability)
		capFilter = &c
	}
	rows, err := db.Query(ctx, `SELECT `+modelColumns+` FROM models
		WHERE tenant_id = $3 AND ($1::uuid IS NULL OR provider_id = $1) AND ($2::text IS NULL OR $2 = ANY(capabilities))
		ORDER BY display_name`, providerID, capFilter, platformTenantID)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func deleteModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	tag, err := db.Exec(ctx, `DELETE FROM models WHERE id = $1 AND tenant_id = $2`, id, platformTenantID)
	if postgres.IsForeignKeyViolation(err) {
		return errModelInUse()
	}
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errModelNotFound()
	}
	return nil
}

func upsertRole(ctx context.Context, db postgres.DBTX, role Role, modelID uuid.UUID) error {
	_, err := db.Exec(ctx, `INSERT INTO model_roles (tenant_id, role, model_id) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, role) DO UPDATE SET model_id = EXCLUDED.model_id, updated_at = now()`, role, modelID, platformTenantID)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

func listRoles(ctx context.Context, db postgres.DBTX) ([]RoleAssignment, error) {
	rows, err := db.Query(ctx, `SELECT role, model_id FROM model_roles WHERE tenant_id = $1 ORDER BY role`, platformTenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	out := []RoleAssignment{}
	for rows.Next() {
		var a RoleAssignment
		if err := rows.Scan(&a.Role, &a.ModelID); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func roleModelID(ctx context.Context, db postgres.DBTX, role Role) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `SELECT model_id FROM model_roles WHERE role = $1 AND tenant_id = $2`, role, platformTenantID).Scan(&id)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("get role: %w", err)
	}
	return id, true, nil
}

func getProviderForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (providerRow, error) {
	if _, err := db.Exec(ctx, `SELECT id FROM providers WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, platformTenantID); err != nil {
		return providerRow{}, fmt.Errorf("lock provider: %w", err)
	}
	return getProvider(ctx, db, id)
}
func getModelForShare(ctx context.Context, db postgres.DBTX, id uuid.UUID) (Model, error) {
	if _, err := db.Exec(ctx, `SELECT id FROM models WHERE id=$1 AND tenant_id=$2 FOR SHARE`, id, platformTenantID); err != nil {
		return Model{}, fmt.Errorf("lock model: %w", err)
	}
	return getModel(ctx, db, id)
}
