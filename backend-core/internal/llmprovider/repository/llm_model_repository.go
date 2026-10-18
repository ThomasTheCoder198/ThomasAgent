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

const llmModelColumns = `id, provider_id, model_ref, display_name, capabilities, context_window, embedding_dims,
	input_price_per_mtok::float8, output_price_per_mtok::float8, source, capabilities_edited`

// LLMModelRepo stores the models of each provider, scoped by the tenant in ctx.
type LLMModelRepo interface {
	UpsertModel(ctx context.Context, db postgres.DBTX, input model.LLMModelInput, source string) (model.LLMModel, bool, error)
	DeleteStaleSyncModels(ctx context.Context, db postgres.DBTX, providerID uuid.UUID, keep []uuid.UUID) error
	GetModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error)
	GetModelForShare(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error)
	GetModelForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error)
	ListModels(ctx context.Context, db postgres.DBTX, providerID *uuid.UUID, capability *model.Capability) ([]model.LLMModel, error)
	DeleteModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) error
	UpdateCapabilities(ctx context.Context, db postgres.DBTX, id uuid.UUID, capabilities []model.Capability) (model.LLMModel, error)
}

// rowLock is a SQL row-locking clause; only the two constants below are ever interpolated.
type rowLock string

const (
	lockShare  rowLock = "FOR SHARE"
	lockUpdate rowLock = "FOR UPDATE"
)

type llmModelRepo struct {
	providers ProviderRepo
}

func NewLLMModelRepo(providers ProviderRepo) LLMModelRepo {
	return llmModelRepo{providers: providers}
}

func scanLLMModel(row pgx.Row) (model.LLMModel, error) {
	var llmModel model.LLMModel
	var capabilities []string
	err := row.Scan(&llmModel.ID, &llmModel.ProviderID, &llmModel.ModelRef, &llmModel.DisplayName, &capabilities, &llmModel.ContextWindow, &llmModel.EmbeddingDims,
		&llmModel.InputPricePerMTok, &llmModel.OutputPricePerMTok, &llmModel.Source, &llmModel.CapabilitiesEdited)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.LLMModel{}, errors.ErrLLMModelNotFound
	}
	if err != nil {
		return model.LLMModel{}, fmt.Errorf("scan model: %w", err)
	}
	llmModel.Capabilities = toCapabilities(capabilities)
	return llmModel, nil
}

func toCapabilities(values []string) []model.Capability {
	capabilities := make([]model.Capability, len(values))
	for index, value := range values {
		capabilities[index] = model.Capability(value)
	}
	return capabilities
}

func capabilityStrings(capabilities []model.Capability) []string {
	values := make([]string, len(capabilities))
	for index, capability := range capabilities {
		values[index] = string(capability)
	}
	return values
}

// UpsertModel reports whether the row was inserted (true) or updated (false); xmax = 0 marks a fresh insert.
// Only a synced row is overwritten by a sync; a manual row with the same ref is reported as ErrLLMNameTaken.
func (repo llmModelRepo) UpsertModel(ctx context.Context, db postgres.DBTX, input model.LLMModelInput, source string) (model.LLMModel, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.LLMModel{}, false, err
	}
	if _, err := repo.providers.GetProvider(ctx, db, input.ProviderID); err != nil {
		return model.LLMModel{}, false, err
	}
	var inserted bool
	row := db.QueryRow(ctx, `INSERT INTO models (tenant_id, provider_id, model_ref, display_name, capabilities, context_window,
		embedding_dims, input_price_per_mtok, output_price_per_mtok, source)
		VALUES ($10,$1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (provider_id, model_ref) DO UPDATE SET display_name = EXCLUDED.display_name,
			capabilities = CASE WHEN models.capabilities_edited THEN models.capabilities ELSE EXCLUDED.capabilities END, context_window = EXCLUDED.context_window,
			embedding_dims = EXCLUDED.embedding_dims, input_price_per_mtok = EXCLUDED.input_price_per_mtok,
			output_price_per_mtok = EXCLUDED.output_price_per_mtok, updated_at = now()
		WHERE models.source = 'sync' AND EXCLUDED.source = 'sync' AND models.tenant_id = EXCLUDED.tenant_id
		RETURNING `+llmModelColumns+`, (xmax = 0)`,
		input.ProviderID, input.ModelRef, input.DisplayName, capabilityStrings(input.Capabilities), input.ContextWindow, input.EmbeddingDims,
		input.InputPricePerMTok, input.OutputPricePerMTok, source, tenantID)
	var llmModel model.LLMModel
	var capabilities []string
	err = row.Scan(&llmModel.ID, &llmModel.ProviderID, &llmModel.ModelRef, &llmModel.DisplayName, &capabilities, &llmModel.ContextWindow, &llmModel.EmbeddingDims,
		&llmModel.InputPricePerMTok, &llmModel.OutputPricePerMTok, &llmModel.Source, &llmModel.CapabilitiesEdited, &inserted)
	if postgres.IsForeignKeyViolation(err) {
		return model.LLMModel{}, false, errors.ErrLLMProviderNotFound
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.LLMModel{}, false, errors.ErrLLMNameTaken
	}
	if err != nil {
		return model.LLMModel{}, false, fmt.Errorf("upsert model: %w", err)
	}
	llmModel.Capabilities = toCapabilities(capabilities)
	return llmModel, inserted, nil
}

// DeleteStaleSyncModels removes synced models missing from the latest catalog, except those a role still uses.
func (llmModelRepo) DeleteStaleSyncModels(ctx context.Context, db postgres.DBTX, providerID uuid.UUID, keep []uuid.UUID) error {
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

func (llmModelRepo) GetModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.LLMModel{}, err
	}
	return scanLLMModel(db.QueryRow(ctx, `SELECT `+llmModelColumns+` FROM models WHERE id = $1 AND tenant_id = $2`, id, tenantID))
}

// GetModelForShare locks the row against deletion while a role is assigned to it.
func (repo llmModelRepo) GetModelForShare(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error) {
	return repo.getModelLocked(ctx, db, id, lockShare)
}

// GetModelForUpdate takes the lock that conflicts with GetModelForShare, so a capability edit and a role assignment of the
// same model serialize; share locks alone are compatible with each other and would let both commit.
func (repo llmModelRepo) GetModelForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.LLMModel, error) {
	return repo.getModelLocked(ctx, db, id, lockUpdate)
}

func (repo llmModelRepo) getModelLocked(ctx context.Context, db postgres.DBTX, id uuid.UUID, lock rowLock) (model.LLMModel, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.LLMModel{}, err
	}
	if _, err = db.Exec(ctx, `SELECT id FROM models WHERE id=$1 AND tenant_id=$2 `+string(lock), id, tenantID); err != nil {
		return model.LLMModel{}, fmt.Errorf("lock model: %w", err)
	}
	return repo.GetModel(ctx, db, id)
}

func (llmModelRepo) ListModels(ctx context.Context, db postgres.DBTX, providerID *uuid.UUID, capability *model.Capability) ([]model.LLMModel, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	var capabilityFilter *string
	if capability != nil {
		value := string(*capability)
		capabilityFilter = &value
	}
	rows, err := db.Query(ctx, `SELECT `+llmModelColumns+` FROM models
		WHERE tenant_id = $3 AND ($1::uuid IS NULL OR provider_id = $1) AND ($2::text IS NULL OR $2 = ANY(capabilities))
		ORDER BY display_name`, providerID, capabilityFilter, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()
	llmModels := []model.LLMModel{}
	for rows.Next() {
		llmModel, err := scanLLMModel(rows)
		if err != nil {
			return nil, err
		}
		llmModels = append(llmModels, llmModel)
	}
	return llmModels, rows.Err()
}

func (llmModelRepo) DeleteModel(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `DELETE FROM models WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if postgres.IsForeignKeyViolation(err) {
		return errors.ErrLLMModelInUse
	}
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrLLMModelNotFound
	}
	return nil
}

// UpdateCapabilities is the operator's edit; capabilities_edited makes every later sync keep it (O8).
func (llmModelRepo) UpdateCapabilities(ctx context.Context, db postgres.DBTX, id uuid.UUID, capabilities []model.Capability) (model.LLMModel, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.LLMModel{}, err
	}
	return scanLLMModel(db.QueryRow(ctx, `UPDATE models SET capabilities = $3, capabilities_edited = true, updated_at = now()
		WHERE id = $1 AND tenant_id = $2 RETURNING `+llmModelColumns, id, tenantID, capabilityStrings(capabilities)))
}
