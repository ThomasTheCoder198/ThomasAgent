package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

type finishingBeginKey struct{}

type finishingRaceRepo struct {
	convrepository.ConversationRepo
	beginHasLock chan struct{}
	releaseBegin chan struct{}
	finishPID    chan int
}

func (r finishingRaceRepo) GetConversationForUpdate(ctx context.Context, db postgres.DBTX, id uuid.UUID) (convmodel.Conversation, error) {
	if ctx.Value(finishingBeginKey{}) == nil {
		var pid int
		if err := db.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return convmodel.Conversation{}, err
		}
		r.finishPID <- pid
		return r.ConversationRepo.GetConversationForUpdate(ctx, db, id)
	}
	conversation, err := r.ConversationRepo.GetConversationForUpdate(ctx, db, id)
	if err != nil {
		return conversation, err
	}
	close(r.beginHasLock)
	<-r.releaseBegin
	return conversation, nil
}

func newChatStore(pool *pgxpool.Pool) convservice.ChatStore {
	return convservice.NewChatStore(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(),
		convrepository.NewRunRepo(), agentrepository.NewAgentRepo(), testFinishTimeout)
}

func seedAgent(t *testing.T, pool *pgxpool.Pool, orgID, name string) uuid.UUID {
	t.Helper()
	agent, err := agentrepository.NewAgentRepo().InsertAgent(orgContext(orgID), pool, agentmodel.Agent{ID: uuid.New(), Name: name, KBID: uuid.New()})
	require.NoError(t, err)
	return agent.ID
}

func userText(id, text string) convmodel.Message {
	return convmodel.Message{ID: id, Role: convmodel.RoleUser, Parts: json.RawMessage(`[{"type":"text","text":"` + text + `"}]`)}
}

func TestChatStore_ConcurrentAgentMismatch(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	first := seedAgent(t, pool, "org-a", "A")
	second := seedAgent(t, pool, "org-a", "B")
	store := newChatStore(pool)
	id := uuid.New()
	ctx := conversationContext("org-a", user)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, agent := range []uuid.UUID{first, second} {
		wg.Add(1)
		go func(agentID uuid.UUID) {
			defer wg.Done()
			<-start
			_, _, err := store.EnsureConversation(ctx, id, agentID, user)
			results <- err
		}(agent)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, mismatches := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.CodeOf(err) == errors.CodeValidationFailed:
			mismatches++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, mismatches)
}

func TestChatStore_SameOrgDifferentUserCannotWrite(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	owner := seedUser(t, pool, "owner@test.local")
	other := seedUser(t, pool, "other@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	id := uuid.New()
	ownerCtx := conversationContext("org-a", owner)
	otherCtx := conversationContext("org-a", other)
	_, _, err := store.EnsureConversation(ownerCtx, id, agent, owner)
	require.NoError(t, err)
	_, _, err = store.EnsureConversation(otherCtx, id, agent, other)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	_, _, err = store.BeginRun(otherCtx, submitInput(id, agent, "u1", "secret", "a1"))
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	_, err = store.LoadHistory(otherCtx, id, 1)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM runs`), "the other user's attempt left no run")
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM outbox`), "nor an outbox row")
	_, _, err = store.BeginRun(ownerCtx, submitInput(id, agent, "u1", "secret", "a1"))
	require.NoError(t, err)
}

func TestChatStore_ConversationLifecycle(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()

	_, _, err := store.EnsureConversation(ctx, conv, uuid.Nil, user)
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err), "a new conversation needs an agent")
	got, created, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, agent, got.AgentID)
	_, created, err = store.EnsureConversation(ctx, conv, uuid.Nil, user)
	require.NoError(t, err)
	require.False(t, created, "nil agent means keep the stored one")
	_, _, err = store.EnsureConversation(ctx, conv, seedAgent(t, pool, "org-a", "B"), user)
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err), "a conversation never changes agent")

	run, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	active, err := store.ActiveRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, active.ID)
	lease := claim(t, store, ctx, run)
	require.NoError(t, store.RecordStep(ctx, lease, convmodel.RunStep{RunID: run.ID, Step: 1, Kind: convmodel.StepKindRouter,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 9000, DurationMS: 5, StartedAt: time.Now()}))
	require.NoError(t, store.RecordStep(ctx, lease, convmodel.RunStep{RunID: run.ID, Step: 2, Kind: convmodel.StepKindModel,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 1200, DurationMS: 5, StartedAt: time.Now()}))
	require.NoError(t, store.RecordStep(ctx, lease, convmodel.RunStep{RunID: run.ID, Step: 3, Kind: convmodel.StepKindModel,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 300, DurationMS: 5, StartedAt: time.Now()}))
	history, err := store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Zero(t, history.PeakInputTokens, "a running run is not finished yet")
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, lease, run, convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"500k [1]"}]`)}))
	_, err = store.ActiveRun(ctx, run.ID)
	require.Equal(t, errors.CodeRunNotActive, errors.CodeOf(err))

	history, err = store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Len(t, history.Messages, 2)
	require.Equal(t, "a1", history.Messages[1].ID)
	require.EqualValues(t, 1200, history.PeakInputTokens, "largest model step of the latest finished run; router steps do not count")

	page, err := store.ListMessagesRange(ctx, conv, 0, history.Messages[1].Seq, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"u1"}, []string{page[0].ID}, "oldest first, capped at limit")
	page, err = store.ListMessagesRange(ctx, conv, history.Messages[0].Seq, history.Messages[1].Seq, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "a1", page[0].ID, "afterSeq is exclusive, throughSeq inclusive")
	page, err = store.ListMessagesRange(conversationContext("org-b", user), conv, 0, history.Messages[1].Seq, 10)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	require.Empty(t, page, "another org never reads these messages")

	require.NoError(t, store.SaveSummary(ctx, conv, "Người dùng hỏi giá.", history.Messages[0].Seq))
	history, err = store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Len(t, history.Messages, 1, "summarized messages are not loaded again (O1)")
	require.Equal(t, "Người dùng hỏi giá.", history.Conversation.Summary)

	_, _, err = store.BeginRun(ctx, regenerateInput(conv, agent, "u1", "a1"))
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err), "only the last message can be regenerated")
	regenerated, _, err := store.BeginRun(ctx, regenerateInput(conv, agent, "a1", "a1"))
	require.NoError(t, err)
	require.NotEqual(t, run.ID, regenerated.ID)
	history, err = store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Empty(t, history.Messages, "u1 is before the summary point; a1 is gone")

	require.NoError(t, store.SetTitle(ctx, conv, "Giá gói Pro"))
}

func TestChatStore_BeginRunExpiresStaleRun(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	agent := seedAgent(t, pool, "org-a", "A")
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	stale, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(err), "a live run blocks a second one")

	_, err = pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - make_interval(secs => $2::float8) WHERE id = $1`, stale.ID, (testFinishTimeout + time.Second).Seconds())
	require.NoError(t, err)
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
	require.NoError(t, err, "a run past its deadline (a crashed replica) never blocks the conversation")
	var status, code string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error_code FROM runs WHERE id = $1`, stale.ID).Scan(&status, &code))
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", code)
}

func TestChatStore_BeginRunPreservesLiveFinishingRun(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	agent := seedAgent(t, pool, "org-a", "A")
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	first, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	lease := claim(t, store, ctx, first)
	_, err = pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second' WHERE id = $1`, first.ID)
	require.NoError(t, err)

	started := make(chan struct{})
	next := make(chan error, 1)
	go func() {
		close(started)
		_, _, beginErr := store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
		next <- beginErr
	}()
	<-started
	beginErr := <-next
	require.Error(t, beginErr, "a live lease has the finish window to save its answer")
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(beginErr))
	first.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, lease, first, convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant,
		Parts: json.RawMessage(`[{"type":"text","text":"partial"}]`)}))
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
	require.NoError(t, err, "the next run starts after the previous answer is saved")
}

func TestChatStore_BeginRunContendsWithFinishRunInsideFinishWindow(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	first, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	lease := claim(t, store, ctx, first)

	const contentionFinishTimeout = time.Minute
	repo := finishingRaceRepo{ConversationRepo: convrepository.NewConversationRepo(), beginHasLock: make(chan struct{}),
		releaseBegin: make(chan struct{}), finishPID: make(chan int, 1)}
	contended := convservice.NewChatStore(pool, repo, convrepository.NewMessageRepo(), convrepository.NewRunRepo(),
		agentrepository.NewAgentRepo(), contentionFinishTimeout)
	defer func() {
		select {
		case <-repo.releaseBegin:
		default:
			close(repo.releaseBegin)
		}
	}()
	beginResult := make(chan error, 1)
	go func() {
		_, _, beginErr := contended.BeginRun(context.WithValue(ctx, finishingBeginKey{}, true), submitInput(conv, agent, "u2", "again", "a2"))
		beginResult <- beginErr
	}()
	<-repo.beginHasLock

	first.Status = convmodel.RunStatusCompleted
	finishResult := make(chan error, 1)
	go func() {
		finishResult <- contended.FinishRun(ctx, lease, first, convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant,
			Parts: json.RawMessage(`[{"type":"text","text":"partial"}]`)})
	}()
	finishPID := <-repo.finishPID
	require.Eventually(t, func() bool {
		var waitType string
		return pool.QueryRow(context.Background(), `SELECT wait_event_type FROM pg_stat_activity WHERE pid = $1`, finishPID).Scan(&waitType) == nil && waitType == "Lock"
	}, 5*time.Second, 10*time.Millisecond, "FinishRun must contend on the conversation row while BeginRun holds it")
	_, err = pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second',
		lease_until = now() + make_interval(secs => $2::float8) WHERE id = $1`, first.ID, contentionFinishTimeout.Seconds())
	require.NoError(t, err)
	close(repo.releaseBegin)

	beginErr := <-beginResult
	require.Error(t, beginErr, "the competing run must remain blocked until the first run finishes")
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(beginErr), "the live lease keeps its finish window while FinishRun waits")
	require.NoError(t, <-finishResult, "the first answer must commit after the waiting finisher gets the row lock")
	second, _, err := store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
	require.NoError(t, err, "the next run can start after the first has finished")
	require.NotEqual(t, first.ID, second.ID)
	var status, answer string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT r.status, m.parts::text FROM runs r JOIN messages m ON m.conversation_id = r.conversation_id AND m.id = 'a1' WHERE r.id = $1`, first.ID).Scan(&status, &answer))
	require.Equal(t, string(convmodel.RunStatusCompleted), status)
	require.JSONEq(t, `[{"type":"text","text":"partial"}]`, answer)
	var startedAfterFinish bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT next.started_at >= first.finished_at FROM runs first JOIN runs next ON next.id = $2 WHERE first.id = $1`,
		first.ID, second.ID).Scan(&startedAfterFinish))
	require.True(t, startedAfterFinish, "the second run starts after the first ends")
}

func TestChatStore_BeginRunInterruptsAnExpiredLeaseBeforeTheFinishWindow(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	agent := seedAgent(t, pool, "org-a", "A")
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	first, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	claim(t, store, ctx, first)
	_, err = pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second', lease_until = now() - interval '1 second' WHERE id = $1`, first.ID)
	require.NoError(t, err)
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u2", "again", "a2"))
	require.NoError(t, err)
	var status, code string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error_code FROM runs WHERE id = $1`, first.ID).Scan(&status, &code))
	require.Equal(t, "failed", status)
	require.Equal(t, string(errors.CodeRunInterrupted), code)
}

func TestChatStore_PeakInputTokensUsesLatestFinishedRun(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	seedReadyOrg(t, pool, "org-b")
	user := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	runs := convrepository.NewRunRepo()
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	agent := seedAgent(t, pool, "org-a", "A")
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	start := func(messageID, assistantID string) (convmodel.Run, convmodel.Lease) {
		run, _ := beginSubmit(t, store, ctx, conv, agent, messageID, assistantID)
		return run, claim(t, store, ctx, run)
	}
	step := func(run convmodel.Run, lease convmodel.Lease, tokens int64) {
		require.NoError(t, store.RecordStep(ctx, lease, convmodel.RunStep{RunID: run.ID, Step: 1, Kind: convmodel.StepKindModel,
			Outcome: convmodel.StepOutcomeSuccess, InputTokens: tokens, DurationMS: 1, StartedAt: time.Now()}))
	}
	peak := func(c context.Context) int64 {
		got, err := runs.PeakInputTokens(c, pool, conv)
		require.NoError(t, err)
		return got
	}

	require.Zero(t, peak(ctx), "no run yet")
	first, firstLease := start("u1", "a1")
	step(first, firstLease, 5000)
	first.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, firstLease, first, convmodel.Message{}))
	require.EqualValues(t, 5000, peak(ctx))

	second, secondLease := start("u2", "a2")
	step(second, secondLease, 700)
	second.Status = convmodel.RunStatusFailed
	require.NoError(t, store.FinishRun(ctx, secondLease, second, convmodel.Message{}))
	require.EqualValues(t, 700, peak(ctx), "the latest finished run wins, not the largest ever")

	third, thirdLease := start("u3", "a3")
	step(third, thirdLease, 99000)
	require.EqualValues(t, 700, peak(ctx), "a running run is not counted")
	require.Zero(t, peak(conversationContext("org-b", user)), "another org never reads these steps")
}

// TestChatStore_BeginRunWritesMessageRunAndOutboxInOneTransaction: the user message, the touch, the run row and the
// chat.runs outbox row commit together, and the rows carry the request id of the call (amendment W3, W9).
func TestChatStore_BeginRunWritesMessageRunAndOutboxInOneTransaction(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx, requestID := withRequestID(conversationContext("org-a", user))
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	var before time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT updated_at FROM conversations WHERE id = $1`, conv).Scan(&before))

	run, outboxID, err := store.BeginRun(ctx, submitInput(conv, agent, "u1", "Giá gói Pro?", "a1"))
	require.NoError(t, err)
	require.Equal(t, convmodel.RunStatusRunning, run.Status)
	require.Equal(t, "a1", run.AssistantMessageID)
	require.Equal(t, requestID, run.RequestID)

	var seq int64
	var role string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT seq, role FROM messages WHERE conversation_id = $1 AND id = 'u1'`, conv).Scan(&seq, &role))
	require.EqualValues(t, 1, seq)
	require.Equal(t, "user", role)
	var after time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT updated_at FROM conversations WHERE id = $1`, conv).Scan(&after))
	require.True(t, after.After(before), "the message touches the conversation")

	var storedRequestID string
	var leaseToken *uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT request_id, lease_token FROM runs WHERE id = $1`, run.ID).Scan(&storedRequestID, &leaseToken))
	require.Equal(t, requestID, storedRequestID, "the run row stores the request id")
	require.Nil(t, leaseToken, "an unclaimed run holds no lease")

	var topic, tenantID, outboxRequestID string
	var payload []byte
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT topic, tenant_id, request_id, payload FROM outbox WHERE id = $1 AND published_at IS NULL`, outboxID).
		Scan(&topic, &tenantID, &outboxRequestID, &payload))
	require.Equal(t, convmodel.StreamChatRuns, topic)
	require.Equal(t, "org-a", tenantID)
	require.Equal(t, requestID, outboxRequestID)
	var job convmodel.ChatRunJob
	require.NoError(t, json.Unmarshal(payload, &job))
	require.Equal(t, convmodel.ChatRunJob{RunID: run.ID, ConversationID: conv, UserID: user}, job)
}

func TestChatStore_BeginRunWithoutRequestIDStillStoresMatchingIDs(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	run, outboxID, err := store.BeginRun(ctx, submitInput(conv, agent, "u1", "hi", "a1"))
	require.NoError(t, err)
	var outboxRequestID string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT request_id FROM outbox WHERE id = $1`, outboxID).Scan(&outboxRequestID))
	require.NotEmpty(t, run.RequestID)
	require.Equal(t, outboxRequestID, run.RequestID, "one id for the run row and the outbox row, even when ctx had none")
}

func TestChatStore_BeginRunRegenerateShapes(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	first, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	lease := claim(t, store, ctx, first)
	first.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, lease, first, convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"x"}]`)}))
	messages := func() int {
		return countRows(t, pool, `SELECT count(*) FROM messages WHERE conversation_id = $1`, conv)
	}
	require.Equal(t, 2, messages())

	// A wrong id is NOT_FOUND and leaves no run or outbox row behind (R6).
	runsBefore, outboxBefore := countRows(t, pool, `SELECT count(*) FROM runs`), countRows(t, pool, `SELECT count(*) FROM outbox`)
	_, _, err = store.BeginRun(ctx, regenerateInput(conv, agent, "u1", "a1"))
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err), "u1 is not the last message")
	_, _, err = store.BeginRun(ctx, regenerateInput(conv, agent, "nope", "a1"))
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err))
	require.Equal(t, runsBefore, countRows(t, pool, `SELECT count(*) FROM runs`))
	require.Equal(t, outboxBefore, countRows(t, pool, `SELECT count(*) FROM outbox`))
	require.Equal(t, 2, messages())

	// The last assistant message is dropped; its run row stays as history.
	second, _, err := store.BeginRun(ctx, regenerateInput(conv, agent, "a1", "a1"))
	require.NoError(t, err)
	require.Equal(t, 1, messages(), "the answer was dropped")
	require.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM runs WHERE conversation_id = $1`, conv))
	secondLease := claim(t, store, ctx, second)
	second.Status = convmodel.RunStatusCancelled
	require.NoError(t, store.FinishRun(ctx, secondLease, second, convmodel.Message{}), "a run may end with no answer saved")

	// The last user message with no answer after it is regenerated without deleting anything (W7).
	third, _, err := store.BeginRun(ctx, regenerateInput(conv, agent, "u1", "a1"))
	require.NoError(t, err)
	require.Equal(t, 1, messages(), "nothing to drop")
	require.Equal(t, convmodel.RunStatusRunning, third.Status)
}

func TestChatStore_BeginRunNeedsAMessageOrARegenerateID(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	_, _, err = store.BeginRun(ctx, convmodel.BeginRunInput{ConversationID: conv, AgentID: agent, Trigger: convmodel.TriggerSubmitMessage,
		AssistantMessageID: "a1", Deadline: time.Now().Add(testRunDeadline)})
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err))
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u1", "hi", ""))
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err), "a run needs the assistant message id it will fill")
	_, _, err = store.BeginRun(ctx, submitInput(conv, seedAgent(t, pool, "org-a", "B"), "u1", "hi", "a1"))
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err), "a conversation never changes agent")
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM runs`))
}

func TestChatStore_BeginRunRejectsASecondActiveRunWithoutSideEffects(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	beginSubmit(t, store, ctx, conv, agent, "u1", "a1")
	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u2", "second", "a2"))
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(err))
	require.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM messages WHERE conversation_id = $1`, conv), "the second message was rolled back")
	require.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM runs`))
	require.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM outbox`))
}

// TestChatStore_BeginRunRollsBackWhenTheOutboxInsertFails: no production caller can leave a run without its job, nor a
// message without its run (amendment W3).
func TestChatStore_BeginRunRollsBackWhenTheOutboxInsertFails(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, user)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `ALTER TABLE outbox ADD CONSTRAINT ck_test_no_chat_runs CHECK (topic <> 'chat.runs')`)
	require.NoError(t, err)

	_, _, err = store.BeginRun(ctx, submitInput(conv, agent, "u1", "hi", "a1"))
	require.Error(t, err)
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM messages`), "the message rolled back")
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM runs`), "the run rolled back")
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM outbox`))
}

func TestChatStore_RunStatusIsScopedToTenantAndUser(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	seedReadyOrg(t, pool, "org-b")
	owner := seedUser(t, pool, "owner@test.local")
	other := seedUser(t, pool, "other@test.local")
	agent := seedAgent(t, pool, "org-a", "A")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", owner)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, agent, owner)
	require.NoError(t, err)
	run, _ := beginSubmit(t, store, ctx, conv, agent, "u1", "a1")

	got, err := store.RunStatus(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, convmodel.RunStatusRunning, got.Status)
	require.Equal(t, run.RequestID, got.RequestID)
	_, err = store.RunStatus(conversationContext("org-a", other), run.ID)
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err), "another user of the same org cannot see the run")
	_, err = store.RunStatus(conversationContext("org-b", owner), run.ID)
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err), "nor can another org")
	_, err = store.RunStatus(ctx, uuid.New())
	require.Equal(t, errors.CodeNotFound, errors.CodeOf(err))
}

// TestChatStore_RunStatusKeepsAnOutageAnOutage: only what the user cannot see is NOT_FOUND; a database failure must
// not turn into a 404 on the stop endpoint.
func TestChatStore_RunStatusKeepsAnOutageAnOutage(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	owner := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	pool.Close()
	_, err := store.RunStatus(conversationContext("org-a", owner), uuid.New())
	require.Error(t, err)
	require.NotEqual(t, errors.CodeNotFound, errors.CodeOf(err))
}
