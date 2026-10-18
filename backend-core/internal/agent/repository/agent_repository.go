package repository

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const agentColumns = `id, name, description, instructions, kb_id, model_overrides, created_at, updated_at`

// AgentRepo stores an org's agents; every method scopes by the tenant in ctx (the org, set by RequireOrg).
type AgentRepo interface {
	InsertAgent(ctx context.Context, db postgres.DBTX, agent model.Agent) (model.Agent, error)
	GetAgent(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Agent, error)
	GetAgentForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Agent, error)
	ListAgents(ctx context.Context, db postgres.DBTX) ([]model.Agent, error)
	UpdateAgent(ctx context.Context, db postgres.DBTX, agent model.Agent) (model.Agent, error)
	DeleteAgent(ctx context.Context, db postgres.DBTX, id uuid.UUID) error
}

type agentRepo struct{}

func NewAgentRepo() AgentRepo { return agentRepo{} }

func scanAgent(row pgx.Row) (model.Agent, error) {
	var agent model.Agent
	var overrides []byte
	err := row.Scan(&agent.ID, &agent.Name, &agent.Description, &agent.Instructions, &agent.KBID, &overrides, &agent.CreatedAt, &agent.UpdatedAt)
	switch {
	case stderrors.Is(err, pgx.ErrNoRows):
		return model.Agent{}, errors.ErrAgentNotFound
	case postgres.IsUniqueViolation(err):
		return model.Agent{}, errors.FieldConflict(model.FieldName)
	case err != nil:
		return model.Agent{}, fmt.Errorf("scan agent: %w", err)
	}
	agent.ModelOverrides, err = decodeOverrides(overrides)
	return agent, err
}

func decodeOverrides(raw []byte) (map[llmmodel.Role]uuid.UUID, error) {
	overrides := map[llmmodel.Role]uuid.UUID{}
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return nil, fmt.Errorf("decode model overrides: %w", err)
	}
	return overrides, nil
}

func encodeOverrides(overrides map[llmmodel.Role]uuid.UUID) ([]byte, error) {
	if overrides == nil {
		overrides = map[llmmodel.Role]uuid.UUID{}
	}
	raw, err := json.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("encode model overrides: %w", err)
	}
	return raw, nil
}

func (agentRepo) InsertAgent(ctx context.Context, db postgres.DBTX, agent model.Agent) (model.Agent, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Agent{}, err
	}
	overrides, err := encodeOverrides(agent.ModelOverrides)
	if err != nil {
		return model.Agent{}, err
	}
	return scanAgent(db.QueryRow(ctx, `INSERT INTO agents (id, tenant_id, name, description, instructions, kb_id, model_overrides)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+agentColumns,
		agent.ID, tenantID, agent.Name, agent.Description, agent.Instructions, agent.KBID, overrides))
}

func (agentRepo) GetAgent(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Agent, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Agent{}, err
	}
	return scanAgent(db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantID))
}

func (agentRepo) GetAgentForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Agent, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Agent{}, err
	}
	return scanAgent(db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenantID))
}

// ListAgents returns the org's agents oldest first, so the first one is the chat default (R12).
func (agentRepo) ListAgents(ctx context.Context, db postgres.DBTX) ([]model.Agent, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT `+agentColumns+` FROM agents WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	agents := []model.Agent{}
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func (agentRepo) UpdateAgent(ctx context.Context, db postgres.DBTX, agent model.Agent) (model.Agent, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Agent{}, err
	}
	overrides, err := encodeOverrides(agent.ModelOverrides)
	if err != nil {
		return model.Agent{}, err
	}
	return scanAgent(db.QueryRow(ctx, `UPDATE agents SET name = $3, description = $4, instructions = $5, kb_id = $6,
		model_overrides = $7, updated_at = now() WHERE id = $1 AND tenant_id = $2 RETURNING `+agentColumns,
		agent.ID, tenantID, agent.Name, agent.Description, agent.Instructions, agent.KBID, overrides))
}

func (agentRepo) DeleteAgent(ctx context.Context, db postgres.DBTX, id uuid.UUID) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `DELETE FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrAgentNotFound
	}
	return nil
}
