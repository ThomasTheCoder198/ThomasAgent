package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	runColumns = `id, conversation_id, assistant_message_id, status, route, model_ref, error_code, input_tokens,
		output_tokens, deadline_at, started_at, finished_at`
	// constraintOneActiveRun is the partial unique index of migration 20261017000001.
	constraintOneActiveRun = "uq_runs_one_active"
)

// RunRepo stores runs and their steps; every method scopes by the tenant in ctx.
type RunRepo interface {
	InsertRun(ctx context.Context, db postgres.DBTX, run model.Run) error
	// ExpireStale marks this conversation's running rows whose deadline passed as failed (R6) and returns how many.
	ExpireStale(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (int64, error)
	GetRun(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Run, error)
	FinishRun(ctx context.Context, db postgres.DBTX, run model.Run) error
	InsertStep(ctx context.Context, db postgres.DBTX, step model.RunStep) error
}

type runRepo struct{}

func NewRunRepo() RunRepo { return runRepo{} }

// InsertRun proves the conversation belongs to the tenant in ctx: another org's conversation is CONVERSATION_NOT_FOUND.
func (runRepo) InsertRun(ctx context.Context, db postgres.DBTX, run model.Run) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `INSERT INTO runs (id, tenant_id, conversation_id, assistant_message_id, status, route, model_ref, deadline_at)
		SELECT $1, c.tenant_id, c.id, $4, $5, $6, $7, $8 FROM conversations c WHERE c.id = $3 AND c.tenant_id = $2`,
		run.ID, tenantID, run.ConversationID, run.AssistantMessageID, run.Status, run.Route, run.ModelRef, run.DeadlineAt)
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) && pgErr.ConstraintName == constraintOneActiveRun {
		return errors.ErrRunAlreadyActive
	}
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrConversationNotFound
	}
	return nil
}

func (runRepo) ExpireStale(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (int64, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	tag, err := db.Exec(ctx, `UPDATE runs SET status = $3, error_code = $4, finished_at = now()
		WHERE conversation_id = $1 AND tenant_id = $2 AND status = 'running' AND deadline_at < now()`,
		conversationID, tenantID, model.RunStatusFailed, string(errors.CodeRunDeadlineExceeded))
	if err != nil {
		return 0, fmt.Errorf("expire stale runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetRun answers RUN_NOT_ACTIVE for another tenant's run as well as a missing one, so a run id never leaks.
func (runRepo) GetRun(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Run, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Run{}, err
	}
	var run model.Run
	err = db.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id = $1 AND tenant_id = $2`, id, tenantID).Scan(
		&run.ID, &run.ConversationID, &run.AssistantMessageID, &run.Status, &run.Route, &run.ModelRef, &run.ErrorCode,
		&run.InputTokens, &run.OutputTokens, &run.DeadlineAt, &run.StartedAt, &run.FinishedAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Run{}, errors.ErrRunNotActive
	}
	if err != nil {
		return model.Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

// FinishRun writes the terminal state once; a run that is no longer running is RUN_NOT_ACTIVE.
func (runRepo) FinishRun(ctx context.Context, db postgres.DBTX, run model.Run) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `UPDATE runs SET status = $3, route = $4, model_ref = $5, error_code = $6, input_tokens = $7,
		output_tokens = $8, finished_at = now() WHERE id = $1 AND tenant_id = $2 AND status = 'running'`,
		run.ID, tenantID, run.Status, run.Route, run.ModelRef, run.ErrorCode, run.InputTokens, run.OutputTokens)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrRunNotActive
	}
	return nil
}

// InsertStep proves the run belongs to the tenant in ctx: another org's run is RUN_NOT_ACTIVE, like GetRun.
func (runRepo) InsertStep(ctx context.Context, db postgres.DBTX, step model.RunStep) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `INSERT INTO run_steps (run_id, step, tenant_id, kind, name, outcome, error_code, input_tokens,
		output_tokens, duration_ms, started_at)
		SELECT r.id, $2, r.tenant_id, $4, $5, $6, $7, $8, $9, $10, $11 FROM runs r WHERE r.id = $1 AND r.tenant_id = $3`,
		step.RunID, step.Step, tenantID, step.Kind, step.Name, step.Outcome, step.ErrorCode, step.InputTokens,
		step.OutputTokens, step.DurationMS, step.StartedAt)
	if err != nil {
		return fmt.Errorf("insert run step: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.ErrRunNotActive
	}
	return nil
}
