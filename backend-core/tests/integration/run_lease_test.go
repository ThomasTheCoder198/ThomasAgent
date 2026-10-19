package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

type leaseFixture struct {
	pool  *pgxpool.Pool
	store convservice.ChatStore
	ctx   context.Context
	conv  uuid.UUID
	agent uuid.UUID
	user  uuid.UUID
}

func newLeaseFixture(t *testing.T) *leaseFixture {
	t.Helper()
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	f := &leaseFixture{pool: pool, store: newChatStore(pool), ctx: conversationContext("org-a", user), conv: uuid.New(),
		agent: seedAgent(t, pool, "org-a", "A"), user: user}
	_, _, err := f.store.EnsureConversation(f.ctx, f.conv, f.agent, user)
	require.NoError(t, err)
	return f
}

// run begins a run in a fresh message slot; finished conversations accept the next one.
func (f *leaseFixture) begin(t *testing.T, tag string) convmodel.Run {
	t.Helper()
	run, _ := beginSubmit(t, f.store, f.ctx, f.conv, f.agent, "u-"+tag, "a-"+tag)
	return run
}

func (f *leaseFixture) step(n int) convmodel.RunStep {
	return convmodel.RunStep{Step: n, Kind: convmodel.StepKindModel, Outcome: convmodel.StepOutcomeSuccess, DurationMS: 1, StartedAt: time.Now()}
}

func (f *leaseFixture) runRow(t *testing.T, id uuid.UUID) (status, code string, token *uuid.UUID, until *time.Time) {
	t.Helper()
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT status, error_code, lease_token, lease_until FROM runs WHERE id = $1`, id).Scan(&status, &code, &token, &until))
	return status, code, token, until
}

// holdRowLock holds the run row the way a concurrent statement would, until release() is called.
func (f *leaseFixture) holdRowLock(t *testing.T, id uuid.UUID) (tx pgx.Tx, release func(commit bool)) {
	t.Helper()
	tx, err := f.pool.Begin(context.Background())
	require.NoError(t, err)
	_, err = tx.Exec(context.Background(), `SELECT 1 FROM runs WHERE id = $1 FOR UPDATE`, id)
	require.NoError(t, err)
	return tx, func(commit bool) {
		if commit {
			require.NoError(t, tx.Commit(context.Background()))
			return
		}
		require.NoError(t, tx.Rollback(context.Background()))
	}
}

func TestRunLease_TwoConcurrentClaimsYieldOneClaimed(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	const claimers = 8
	results := make(chan convmodel.ClaimResult, claimers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range claimers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-"+string(rune('a'+i)), testRunLease)
			if err != nil {
				t.Error(err)
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	claimed, held := 0, 0
	for result := range results {
		switch result {
		case convmodel.ClaimResultClaimed:
			claimed++
		case convmodel.ClaimResultHeldElsewhere:
			held++
		default:
			t.Errorf("unexpected claim result %q", result)
		}
	}
	require.Equal(t, 1, claimed, "exactly one executor")
	require.Equal(t, claimers-1, held)
}

func TestRunLease_ClaimRecordsOwnerTokenAndQueueTime(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET started_at = now() - interval '3 seconds' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	claimed, lease, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-a", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultClaimed, result)
	require.Equal(t, "worker-a", lease.Owner)
	require.GreaterOrEqual(t, claimed.QueuedFor, 3*time.Second, "queue time comes from the database clock")
	var owner string
	var token uuid.UUID
	var until time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT lease_owner, lease_token, lease_until FROM runs WHERE id = $1`, run.ID).Scan(&owner, &token, &until))
	require.Equal(t, "worker-a", owner)
	require.Equal(t, lease.Token, token)
	require.True(t, until.After(time.Now().Add(testRunLease/2)))
}

func TestRunLease_ClaimWhileHeldIsHeldElsewhere(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	first := claim(t, f.store, f.ctx, run)
	_, lease, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-b", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultHeldElsewhere, result)
	require.Equal(t, uuid.Nil, lease.Token, "a refused claim carries no token")
	_, _, token, _ := f.runRow(t, run.ID)
	require.Equal(t, first.Token, *token, "the holder keeps its token")
}

func TestRunLease_ExpiredLeaseIsReportedAndFailRunFlipsItOnce(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	claim(t, f.store, f.ctx, run)
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)

	_, _, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-b", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultExpired, result, "its executor is gone: never re-execute")
	flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
	require.NoError(t, err)
	require.True(t, flipped)
	again, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
	require.NoError(t, err)
	require.False(t, again, "the transition happens once, so the terminal event is written once")
	status, code, token, until := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_INTERRUPTED", code)
	require.Nil(t, token, "the lease is cleared")
	require.Nil(t, until)
	_, _, result, err = f.store.ClaimRun(f.ctx, run.ID, "worker-c", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultNotRunning, result)
}

func TestRunLease_FailRunOnlyIfExpiredLeavesALiveLease(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
	require.NoError(t, err)
	require.False(t, flipped, "a live executor is not interrupted")
	require.NoError(t, f.store.RenewLease(f.ctx, lease, testRunLease))
	status, _, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "running", status)
}

func TestRunLease_QueuedPastDeadline(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	_, lease, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-a", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultQueuedPastDeadline, result)
	require.Equal(t, uuid.Nil, lease.Token)
	flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunDeadlineExceeded), false)
	require.NoError(t, err)
	require.True(t, flipped)
	status, code, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", code)
}

func TestRunLease_FinishedRunIsNotRunning(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{}))
	_, _, result, err := f.store.ClaimRun(f.ctx, run.ID, "worker-b", testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultNotRunning, result)
	_, _, token, until := f.runRow(t, run.ID)
	require.Nil(t, token, "FinishRun clears the lease")
	require.Nil(t, until)
}

func TestRunLease_RenewExtendsTheLease(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() + interval '2 seconds' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	require.NoError(t, f.store.RenewLease(f.ctx, lease, time.Hour))
	_, _, _, until := f.runRow(t, run.ID)
	require.True(t, until.After(time.Now().Add(30*time.Minute)))
}

// TestRunLease_StaleWritersAreFencedOut: a token that is wrong, or whose lease expired, writes nothing at all.
func TestRunLease_StaleWritersAreFencedOut(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	stolen := convmodel.Lease{RunID: run.ID, Token: uuid.New(), Owner: "intruder"}
	assistant := convmodel.Message{ID: "a-1", Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"x"}]`)}
	run.Status = convmodel.RunStatusCompleted

	for name, bad := range map[string]convmodel.Lease{"wrong token": stolen} {
		t.Run(name, func(t *testing.T) {
			step := f.step(1)
			step.RunID = run.ID
			require.ErrorIs(t, f.store.RecordStep(f.ctx, bad, step), errors.ErrRunLeaseLost)
			require.ErrorIs(t, f.store.RenewLease(f.ctx, bad, testRunLease), errors.ErrRunLeaseLost)
			require.ErrorIs(t, f.store.FinishRun(f.ctx, bad, run, assistant), errors.ErrRunLeaseLost)
		})
	}
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM run_steps`), "nothing was written")
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE id = 'a-1'`), "the answer was not stored")
	status, _, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "running", status)

	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	step := f.step(1)
	step.RunID = run.ID
	require.ErrorIs(t, f.store.RecordStep(f.ctx, lease, step), errors.ErrRunLeaseLost, "an expired lease cannot write even with its own token")
	require.ErrorIs(t, f.store.RenewLease(f.ctx, lease, testRunLease), errors.ErrRunLeaseLost, "nor be renewed")
	require.ErrorIs(t, f.store.FinishRun(f.ctx, lease, run, assistant), errors.ErrRunLeaseLost)
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM run_steps`))
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE id = 'a-1'`))
}

// TestRunLease_ExpiryRacesWriters: the row lock each fenced statement takes serializes it with the expiry
// transition, so one side wins and never both. The race is made deterministic by holding the row lock while the
// writer is already waiting on it.
func TestRunLease_ExpiryRacesWriters(t *testing.T) {
	type writer struct {
		name string
		call func(f *leaseFixture, lease convmodel.Lease, run convmodel.Run) error
	}
	writers := []writer{
		{"heartbeat", func(f *leaseFixture, lease convmodel.Lease, _ convmodel.Run) error {
			return f.store.RenewLease(f.ctx, lease, testRunLease)
		}},
		{"RecordStep", func(f *leaseFixture, lease convmodel.Lease, run convmodel.Run) error {
			step := f.step(1)
			step.RunID = run.ID
			return f.store.RecordStep(f.ctx, lease, step)
		}},
		{"FinishRun", func(f *leaseFixture, lease convmodel.Lease, run convmodel.Run) error {
			run.Status = convmodel.RunStatusCompleted
			return f.store.FinishRun(f.ctx, lease, run, convmodel.Message{ID: "a-" + run.ID.String(), Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"x"}]`)})
		}},
	}
	for _, w := range writers {
		t.Run(w.name+" loses when the lease expires first", func(t *testing.T) {
			f := newLeaseFixture(t)
			run := f.begin(t, "1")
			lease := claim(t, f.store, f.ctx, run)
			tx, release := f.holdRowLock(t, run.ID)
			done := make(chan error, 1)
			go func() { done <- w.call(f, lease, run) }()
			time.Sleep(300 * time.Millisecond) // the writer is now waiting for the row lock
			_, err := tx.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
			require.NoError(t, err)
			release(true)
			require.ErrorIs(t, <-done, errors.ErrRunLeaseLost)
			flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
			require.NoError(t, err)
			require.True(t, flipped, "the expiry side won: the run is interrupted")
			require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM run_steps`))
			require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE role = 'assistant'`))
		})
		t.Run(w.name+" wins when the heartbeat extended first", func(t *testing.T) {
			f := newLeaseFixture(t)
			run := f.begin(t, "1")
			lease := claim(t, f.store, f.ctx, run)
			_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() + interval '1 second' WHERE id = $1`, run.ID)
			require.NoError(t, err)
			tx, release := f.holdRowLock(t, run.ID)
			done := make(chan error, 1)
			go func() { done <- w.call(f, lease, run) }()
			time.Sleep(300 * time.Millisecond)
			_, err = tx.Exec(context.Background(), `UPDATE runs SET lease_until = now() + interval '1 hour' WHERE id = $1`, run.ID)
			require.NoError(t, err)
			release(true)
			require.NoError(t, <-done)
			flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
			require.NoError(t, err)
			require.False(t, flipped, "the writer side won: nothing is left to interrupt")
		})
	}
}

// TestRunLease_FailRunRacesFinishRun: when the interrupted transition commits first, the executor's FinishRun is
// fenced out; when FinishRun commits first, the transition changes nothing.
func TestRunLease_FailRunRacesFinishRun(t *testing.T) {
	f := newLeaseFixture(t)
	run := f.begin(t, "1")
	lease := claim(t, f.store, f.ctx, run)
	_, err := f.pool.Exec(context.Background(), `UPDATE runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run.ID)
	require.NoError(t, err)
	flipped, err := f.store.FailRun(f.ctx, run.ID, string(errors.CodeRunInterrupted), true)
	require.NoError(t, err)
	require.True(t, flipped)
	run.Status = convmodel.RunStatusCompleted
	require.ErrorIs(t, f.store.FinishRun(f.ctx, lease, run, convmodel.Message{ID: "a-1", Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"x"}]`)}), errors.ErrRunLeaseLost)
	require.Zero(t, countRows(t, f.pool, `SELECT count(*) FROM messages WHERE role = 'assistant'`), "the interrupted run stores no answer")
	status, code, _, _ := f.runRow(t, run.ID)
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_INTERRUPTED", code)
}
