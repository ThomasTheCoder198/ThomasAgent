package repository

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

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
		output_tokens, deadline_at, started_at, finished_at, request_id`
	// constraintOneActiveRun is the partial unique index of migration 20261017000001.
	constraintOneActiveRun = "uq_runs_one_active"
)

// RunRepo stores runs and their steps; every method scopes by the tenant in ctx.
type RunRepo interface {
	InsertRun(ctx context.Context, db postgres.DBTX, run model.Run) error
	// ExpireStale keeps a live executor through the finish window and reports how many rows it ended.
	ExpireStale(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, finishTimeout time.Duration) (int64, error)
	GetRun(ctx context.Context, db postgres.DBTX, id uuid.UUID) (model.Run, error)
	FinishRun(ctx context.Context, db postgres.DBTX, run model.Run) error
	InsertStep(ctx context.Context, db postgres.DBTX, step model.RunStep) error
	// LatestRun is the conversation's newest run, ok=false when it has none.
	LatestRun(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (model.Run, bool, error)
	// ClaimRun takes the lease of a running, unclaimed run before its deadline; ok=false when the row did not change.
	ClaimRun(ctx context.Context, db postgres.DBTX, runID uuid.UUID, lease model.Lease, d time.Duration) (model.Run, bool, error)
	// ClaimState explains a refused claim from one read (amendment W5).
	ClaimState(ctx context.Context, db postgres.DBTX, runID uuid.UUID) (ClaimState, error)
	// RenewLease extends a live lease; ok=false when the token no longer holds one.
	RenewLease(ctx context.Context, db postgres.DBTX, lease model.Lease, d time.Duration) (bool, error)
	// InsertFencedStep writes a step only while the lease is live, holding the run row for the statement.
	InsertFencedStep(ctx context.Context, db postgres.DBTX, lease model.Lease, step model.RunStep) (bool, error)
	// FinishFencedRun writes the terminal state and clears the lease only while the lease is live.
	FinishFencedRun(ctx context.Context, db postgres.DBTX, lease model.Lease, run model.Run) (bool, error)
	// FailRun is the interrupted / queued-past-deadline transition; onlyIfExpired also needs a lapsed lease.
	FailRun(ctx context.Context, db postgres.DBTX, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error)
	// PeakInputTokens is the largest input_tokens of a model step in the conversation's latest finished run, 0 when
	// there is none.
	PeakInputTokens(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (int64, error)
}

type runRepo struct{}

func NewRunRepo() RunRepo { return runRepo{} }

// InsertRun proves the conversation belongs to the tenant in ctx: another org's conversation is CONVERSATION_NOT_FOUND.
func (runRepo) InsertRun(ctx context.Context, db postgres.DBTX, run model.Run) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `INSERT INTO runs (id, tenant_id, conversation_id, assistant_message_id, status, route, model_ref, deadline_at, request_id)
		SELECT $1, c.tenant_id, c.id, $4, $5, $6, $7, $8, $9 FROM conversations c WHERE c.id = $3 AND c.tenant_id = $2`,
		run.ID, tenantID, run.ConversationID, run.AssistantMessageID, run.Status, run.Route, run.ModelRef, run.DeadlineAt, run.RequestID)
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

func (runRepo) ExpireStale(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID, finishTimeout time.Duration) (int64, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	// A lapsed lease means the executor is gone; a live lease can still save a partial answer until the finish window closes.
	tag, err := db.Exec(ctx, `UPDATE runs SET status = $3,
		error_code = CASE WHEN lease_token IS NOT NULL AND lease_until <= now() THEN $4 ELSE $5 END,
		finished_at = now(), lease_token = NULL, lease_until = NULL
		WHERE conversation_id = $1 AND tenant_id = $2 AND status = 'running'
				AND ((lease_token IS NOT NULL AND lease_until <= now()) OR deadline_at + make_interval(secs => $6::float8) < now())`,
		conversationID, tenantID, model.RunStatusFailed, string(errors.CodeRunInterrupted), string(errors.CodeRunDeadlineExceeded), finishTimeout.Seconds())
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
	err = scanRun(db.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id = $1 AND tenant_id = $2`, id, tenantID), &run)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Run{}, errors.ErrRunNotActive
	}
	if err != nil {
		return model.Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

func scanRun(row pgx.Row, run *model.Run) error {
	return row.Scan(&run.ID, &run.ConversationID, &run.AssistantMessageID, &run.Status, &run.Route, &run.ModelRef, &run.ErrorCode,
		&run.InputTokens, &run.OutputTokens, &run.DeadlineAt, &run.StartedAt, &run.FinishedAt, &run.RequestID)
}

// FinishRun writes the terminal state once; a run that is no longer running is RUN_NOT_ACTIVE.
func (runRepo) FinishRun(ctx context.Context, db postgres.DBTX, run model.Run) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `UPDATE runs SET status = $3, route = $4, model_ref = $5, error_code = $6, input_tokens = $7,
		output_tokens = $8, finished_at = now(), lease_token = NULL, lease_until = NULL WHERE id = $1 AND tenant_id = $2 AND status = 'running'`,
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

func (runRepo) PeakInputTokens(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (int64, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	var peak int64
	err = db.QueryRow(ctx, `SELECT COALESCE(MAX(s.input_tokens), 0) FROM run_steps s
		WHERE s.tenant_id = $2 AND s.kind = $3 AND s.run_id = (
			SELECT r.id FROM runs r WHERE r.conversation_id = $1 AND r.tenant_id = $2 AND r.status <> $4
			ORDER BY r.finished_at DESC NULLS LAST, r.started_at DESC LIMIT 1)`,
		conversationID, tenantID, model.StepKindModel, model.RunStatusRunning).Scan(&peak)
	if err != nil {
		return 0, fmt.Errorf("peak input tokens: %w", err)
	}
	return peak, nil
}

// ClaimState is what a refused claim reads back; lease times are compared with the database clock.
type ClaimState struct {
	Found        bool
	Status       model.RunStatus
	LeaseHeld    bool
	LeaseLapsed  bool
	PastDeadline bool
	DeadlineAt   time.Time
}

// ClaimRun is the attempt-unique claim: a fresh token per attempt, tenant and conversation owner both checked.
func (runRepo) ClaimRun(ctx context.Context, db postgres.DBTX, runID uuid.UUID, lease model.Lease, d time.Duration) (model.Run, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Run{}, false, err
	}
	userID, err := authenticatedUserID(ctx)
	if err != nil {
		return model.Run{}, false, err
	}
	var run model.Run
	var queuedSeconds float64
	err = db.QueryRow(ctx, `UPDATE runs SET lease_owner = $4, lease_token = $5, lease_until = now() + make_interval(secs => $6::float8)
		WHERE id = $1 AND tenant_id = $2 AND status = 'running' AND lease_token IS NULL AND deadline_at > now()
			AND EXISTS (SELECT 1 FROM conversations c WHERE c.id = runs.conversation_id AND c.tenant_id = $2 AND c.user_id = $3)
		RETURNING `+runColumns+`, EXTRACT(EPOCH FROM now() - started_at)::float8`,
		runID, tenantID, userID, lease.Owner, lease.Token, d.Seconds()).Scan(
		&run.ID, &run.ConversationID, &run.AssistantMessageID, &run.Status, &run.Route, &run.ModelRef, &run.ErrorCode,
		&run.InputTokens, &run.OutputTokens, &run.DeadlineAt, &run.StartedAt, &run.FinishedAt, &run.RequestID, &queuedSeconds)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Run{}, false, nil
	}
	if err != nil {
		return model.Run{}, false, fmt.Errorf("claim run: %w", err)
	}
	run.QueuedFor = time.Duration(queuedSeconds * float64(time.Second))
	return run, true, nil
}

func (runRepo) ClaimState(ctx context.Context, db postgres.DBTX, runID uuid.UUID) (ClaimState, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return ClaimState{}, err
	}
	state := ClaimState{Found: true}
	err = db.QueryRow(ctx, `SELECT status, lease_token IS NOT NULL, COALESCE(lease_until <= now(), false), deadline_at <= now(), deadline_at
		FROM runs WHERE id = $1 AND tenant_id = $2`, runID, tenantID).Scan(&state.Status, &state.LeaseHeld, &state.LeaseLapsed, &state.PastDeadline, &state.DeadlineAt)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return ClaimState{}, nil
	}
	if err != nil {
		return ClaimState{}, fmt.Errorf("read claim state: %w", err)
	}
	return state, nil
}

func (runRepo) RenewLease(ctx context.Context, db postgres.DBTX, lease model.Lease, d time.Duration) (bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return false, err
	}
	tag, err := db.Exec(ctx, `UPDATE runs SET lease_until = now() + make_interval(secs => $4::float8)
		WHERE id = $1 AND tenant_id = $2 AND lease_token = $3 AND status = 'running' AND lease_until > now()`,
		lease.RunID, tenantID, lease.Token, d.Seconds())
	if err != nil {
		return false, fmt.Errorf("renew lease: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// InsertFencedStep takes the run row lock for the statement, so it serializes with the expiry transition.
func (runRepo) InsertFencedStep(ctx context.Context, db postgres.DBTX, lease model.Lease, step model.RunStep) (bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return false, err
	}
	tag, err := db.Exec(ctx, `INSERT INTO run_steps (run_id, step, tenant_id, kind, name, outcome, error_code, input_tokens,
		output_tokens, duration_ms, started_at)
		SELECT r.id, $3, r.tenant_id, $5, $6, $7, $8, $9, $10, $11, $12 FROM runs r
		WHERE r.id = $1 AND r.tenant_id = $2 AND r.lease_token = $4 AND r.status = 'running' AND r.lease_until > now()
		FOR UPDATE OF r`,
		lease.RunID, tenantID, step.Step, lease.Token, step.Kind, step.Name, step.Outcome, step.ErrorCode, step.InputTokens,
		step.OutputTokens, step.DurationMS, step.StartedAt)
	if err != nil {
		return false, fmt.Errorf("insert fenced run step: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (runRepo) FinishFencedRun(ctx context.Context, db postgres.DBTX, lease model.Lease, run model.Run) (bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return false, err
	}
	tag, err := db.Exec(ctx, `UPDATE runs SET status = $4, route = $5, model_ref = $6, error_code = $7, input_tokens = $8,
		output_tokens = $9, finished_at = now(), lease_token = NULL, lease_until = NULL
		WHERE id = $1 AND tenant_id = $2 AND lease_token = $3 AND status = 'running' AND lease_until > now()`,
		lease.RunID, tenantID, lease.Token, run.Status, run.Route, run.ModelRef, run.ErrorCode, run.InputTokens, run.OutputTokens)
	if err != nil {
		return false, fmt.Errorf("finish fenced run: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (runRepo) FailRun(ctx context.Context, db postgres.DBTX, runID uuid.UUID, code string, onlyIfExpired bool) (bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return false, err
	}
	query := `UPDATE runs SET status = 'failed', error_code = $3, finished_at = now(), lease_token = NULL, lease_until = NULL
		WHERE id = $1 AND tenant_id = $2 AND status = 'running'`
	if onlyIfExpired {
		query += ` AND lease_token IS NOT NULL AND lease_until <= now()`
	}
	tag, err := db.Exec(ctx, query, runID, tenantID, code)
	if err != nil {
		return false, fmt.Errorf("fail run: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (runRepo) LatestRun(ctx context.Context, db postgres.DBTX, conversationID uuid.UUID) (model.Run, bool, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.Run{}, false, err
	}
	var run model.Run
	err = scanRun(db.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE conversation_id = $1 AND tenant_id = $2
		ORDER BY started_at DESC, id DESC LIMIT 1`, conversationID, tenantID), &run)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.Run{}, false, nil
	}
	if err != nil {
		return model.Run{}, false, fmt.Errorf("latest run: %w", err)
	}
	return run, true, nil
}
