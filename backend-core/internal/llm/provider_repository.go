package llm

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const providerColumns = `id, kind, name, base_url, api_key_secret_id IS NOT NULL, enabled, created_at, updated_at`

func scanProvider(row pgx.Row) (Provider, error) {
	var provider Provider
	err := row.Scan(&provider.ID, &provider.Kind, &provider.Name, &provider.BaseURL, &provider.HasAPIKey, &provider.Enabled, &provider.CreatedAt, &provider.UpdatedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return Provider{}, errors.ErrLLMProviderNotFound
	}
	if err != nil {
		return Provider{}, fmt.Errorf("scan provider: %w", err)
	}
	return provider, nil
}

func (Repository) InsertProvider(ctx context.Context, db postgres.DBTX, input ProviderInput, secretID *uuid.UUID) (Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return Provider{}, err
	}
	enabled := input.Enabled == nil || *input.Enabled
	provider, err := scanProvider(db.QueryRow(ctx, `INSERT INTO providers (tenant_id, kind, name, base_url, api_key_secret_id, enabled)
		VALUES ($6, $1, $2, $3, $4, $5) RETURNING `+providerColumns, input.Kind, input.Name, input.BaseURL, secretID, enabled, tenantID))
	if postgres.IsUniqueViolation(err) {
		return Provider{}, errors.ErrLLMNameTaken
	}
	return provider, err
}

func (Repository) GetProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (storedProvider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return storedProvider{}, err
	}
	var stored storedProvider
	err = db.QueryRow(ctx, `SELECT `+providerColumns+`, api_key_secret_id FROM providers WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&stored.ID, &stored.Kind, &stored.Name, &stored.BaseURL, &stored.HasAPIKey, &stored.Enabled, &stored.CreatedAt, &stored.UpdatedAt, &stored.SecretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return storedProvider{}, errors.ErrLLMProviderNotFound
	}
	if err != nil {
		return storedProvider{}, fmt.Errorf("get provider: %w", err)
	}
	return stored, nil
}

// GetProviderForUpdate locks the row so concurrent updates cannot race on the vault secret.
func (repository Repository) GetProviderForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (storedProvider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return storedProvider{}, err
	}
	if _, err = db.Exec(ctx, `SELECT id FROM providers WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, tenantID); err != nil {
		return storedProvider{}, fmt.Errorf("lock provider: %w", err)
	}
	return repository.GetProvider(ctx, db, id)
}

func (Repository) UpdateProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID, name, baseURL string, enabled bool, secretID *uuid.UUID) (Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return Provider{}, err
	}
	provider, err := scanProvider(db.QueryRow(ctx, `UPDATE providers SET name = $2, base_url = $3, enabled = $4,
		api_key_secret_id = $5, updated_at = now() WHERE id = $1 AND tenant_id = $6 RETURNING `+providerColumns, id, name, baseURL, enabled, secretID, tenantID))
	if postgres.IsUniqueViolation(err) {
		return Provider{}, errors.ErrLLMNameTaken
	}
	return provider, err
}

// DeleteProvider returns the vault secret id so the caller can delete the key in the same transaction.
func (Repository) DeleteProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (*uuid.UUID, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var secretID *uuid.UUID
	err = db.QueryRow(ctx, `DELETE FROM providers WHERE id = $1 AND tenant_id = $2 RETURNING api_key_secret_id`, id, tenantID).Scan(&secretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return nil, errors.ErrLLMProviderNotFound
	}
	if postgres.IsForeignKeyViolation(err) {
		return nil, errors.ErrLLMModelInUse
	}
	if err != nil {
		return nil, fmt.Errorf("delete provider: %w", err)
	}
	return secretID, nil
}

func (Repository) ListProviders(ctx context.Context, db postgres.DBTX) ([]Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+providerColumns+` FROM providers WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	providers := []Provider{}
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, rows.Err()
}
