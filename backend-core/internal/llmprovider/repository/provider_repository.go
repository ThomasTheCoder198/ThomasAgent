package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const providerColumns = `id, kind, name, base_url, api_key_secret_id IS NOT NULL, enabled, created_at, updated_at`

// ProviderRepo stores providers. Every method takes the pool or the service's transaction
// and scopes rows by the tenant in ctx.
type ProviderRepo interface {
	InsertProvider(ctx context.Context, db postgres.DBTX, input model.ProviderInput, secretID *uuid.UUID) (model.Provider, error)
	GetProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.StoredProvider, error)
	GetProviderForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.StoredProvider, error)
	UpdateProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID, name, baseURL string, enabled bool, secretID *uuid.UUID) (model.Provider, error)
	DeleteProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (*uuid.UUID, error)
	ListProviders(ctx context.Context, db postgres.DBTX) ([]model.Provider, error)
}

type providerRepo struct{}

func NewProviderRepo() ProviderRepo { return providerRepo{} }

func scanProvider(row pgx.Row) (model.Provider, error) {
	var provider model.Provider
	err := row.Scan(&provider.ID, &provider.Kind, &provider.Name, &provider.BaseURL, &provider.HasAPIKey, &provider.Enabled, &provider.CreatedAt, &provider.UpdatedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Provider{}, errors.ErrLLMProviderNotFound
	}
	if err != nil {
		return model.Provider{}, fmt.Errorf("scan provider: %w", err)
	}
	return provider, nil
}

func (providerRepo) InsertProvider(ctx context.Context, db postgres.DBTX, input model.ProviderInput, secretID *uuid.UUID) (model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Provider{}, err
	}
	enabled := input.Enabled == nil || *input.Enabled
	provider, err := scanProvider(db.QueryRow(ctx, `INSERT INTO providers (tenant_id, kind, name, base_url, api_key_secret_id, enabled)
		VALUES ($6, $1, $2, $3, $4, $5) RETURNING `+providerColumns, input.Kind, input.Name, input.BaseURL, secretID, enabled, tenantID))
	if postgres.IsUniqueViolation(err) {
		return model.Provider{}, errors.ErrLLMNameTaken
	}
	return provider, err
}

func (providerRepo) GetProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.StoredProvider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.StoredProvider{}, err
	}
	var stored model.StoredProvider
	err = db.QueryRow(ctx, `SELECT `+providerColumns+`, api_key_secret_id FROM providers WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&stored.ID, &stored.Kind, &stored.Name, &stored.BaseURL, &stored.HasAPIKey, &stored.Enabled, &stored.CreatedAt, &stored.UpdatedAt, &stored.SecretID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.StoredProvider{}, errors.ErrLLMProviderNotFound
	}
	if err != nil {
		return model.StoredProvider{}, fmt.Errorf("get provider: %w", err)
	}
	return stored, nil
}

// GetProviderForUpdate locks the row so concurrent updates cannot race on the vault secret.
func (repo providerRepo) GetProviderForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.StoredProvider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.StoredProvider{}, err
	}
	if _, err = db.Exec(ctx, `SELECT id FROM providers WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, tenantID); err != nil {
		return model.StoredProvider{}, fmt.Errorf("lock provider: %w", err)
	}
	return repo.GetProvider(ctx, db, id)
}

func (providerRepo) UpdateProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID, name, baseURL string, enabled bool, secretID *uuid.UUID) (model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Provider{}, err
	}
	provider, err := scanProvider(db.QueryRow(ctx, `UPDATE providers SET name = $2, base_url = $3, enabled = $4,
		api_key_secret_id = $5, updated_at = now() WHERE id = $1 AND tenant_id = $6 RETURNING `+providerColumns, id, name, baseURL, enabled, secretID, tenantID))
	if postgres.IsUniqueViolation(err) {
		return model.Provider{}, errors.ErrLLMNameTaken
	}
	return provider, err
}

// DeleteProvider returns the vault secret id so the service can delete the key in the same transaction.
func (providerRepo) DeleteProvider(ctx context.Context, db postgres.DBTX, id uuid.UUID) (*uuid.UUID, error) {
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

func (providerRepo) ListProviders(ctx context.Context, db postgres.DBTX) ([]model.Provider, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+providerColumns+` FROM providers WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	providers := []model.Provider{}
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, rows.Err()
}
