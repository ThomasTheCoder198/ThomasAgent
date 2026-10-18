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
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

func newChatStore(pool *pgxpool.Pool) convservice.ChatStore {
	return convservice.NewChatStore(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(),
		convrepository.NewRunRepo(), agentrepository.NewAgentRepo())
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
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(store.AppendUserMessage(otherCtx, id, userText("u1", "secret"))))
	_, err = store.LoadHistory(otherCtx, id, 1)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	require.NoError(t, store.AppendUserMessage(ownerCtx, id, userText("u1", "secret")))
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

	require.NoError(t, store.AppendUserMessage(ctx, conv, userText("u1", "Giá gói Pro?")))
	run := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "a1", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.NoError(t, store.StartRun(ctx, run))
	active, err := store.ActiveRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, active.ID)
	require.NoError(t, store.RecordStep(ctx, convmodel.RunStep{RunID: run.ID, Step: 1, Kind: convmodel.StepKindRouter,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 9000, DurationMS: 5, StartedAt: time.Now()}))
	require.NoError(t, store.RecordStep(ctx, convmodel.RunStep{RunID: run.ID, Step: 2, Kind: convmodel.StepKindModel,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 1200, DurationMS: 5, StartedAt: time.Now()}))
	require.NoError(t, store.RecordStep(ctx, convmodel.RunStep{RunID: run.ID, Step: 3, Kind: convmodel.StepKindModel,
		Outcome: convmodel.StepOutcomeSuccess, InputTokens: 300, DurationMS: 5, StartedAt: time.Now()}))
	history, err := store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Zero(t, history.PeakInputTokens, "a running run is not finished yet")
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, run, convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant, Parts: json.RawMessage(`[{"type":"text","text":"500k [1]"}]`)}))
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

	require.Equal(t, errors.CodeNotFound, errors.CodeOf(store.DropLastAssistant(ctx, conv, "u1")), "only the last assistant message can be dropped")
	require.NoError(t, store.DropLastAssistant(ctx, conv, "a1"))
	history, err = store.LoadHistory(ctx, conv, 10)
	require.NoError(t, err)
	require.Empty(t, history.Messages, "u1 is before the summary point; a1 is gone")

	require.NoError(t, store.SetTitle(ctx, conv, "Giá gói Pro"))
}

func TestChatStore_StartRunExpiresStaleRun(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	user := seedUser(t, pool, "owner@test.local")
	store := newChatStore(pool)
	ctx := conversationContext("org-a", user)
	conv := uuid.New()
	_, _, err := store.EnsureConversation(ctx, conv, seedAgent(t, pool, "org-a", "A"), user)
	require.NoError(t, err)
	stale := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "a1", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.NoError(t, store.StartRun(ctx, stale))
	second := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "a2", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(store.StartRun(ctx, second)), "a live run blocks a second one")

	_, err = pool.Exec(context.Background(), `UPDATE runs SET deadline_at = now() - interval '1 second' WHERE id = $1`, stale.ID)
	require.NoError(t, err)
	require.NoError(t, store.StartRun(ctx, second), "a run past its deadline (a crashed replica) never blocks the conversation")
	var status, code string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status, error_code FROM runs WHERE id = $1`, stale.ID).Scan(&status, &code))
	require.Equal(t, "failed", status)
	require.Equal(t, "RUN_DEADLINE_EXCEEDED", code)
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
	_, _, err := store.EnsureConversation(ctx, conv, seedAgent(t, pool, "org-a", "A"), user)
	require.NoError(t, err)
	step := func(run convmodel.Run, tokens int64) {
		require.NoError(t, store.RecordStep(ctx, convmodel.RunStep{RunID: run.ID, Step: 1, Kind: convmodel.StepKindModel,
			Outcome: convmodel.StepOutcomeSuccess, InputTokens: tokens, DurationMS: 1, StartedAt: time.Now()}))
	}
	start := func(assistantID string) convmodel.Run {
		run := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: assistantID, Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
		require.NoError(t, store.StartRun(ctx, run))
		return run
	}
	peak := func(c context.Context) int64 {
		got, err := runs.PeakInputTokens(c, pool, conv)
		require.NoError(t, err)
		return got
	}

	require.Zero(t, peak(ctx), "no run yet")
	first := start("a1")
	step(first, 5000)
	first.Status = convmodel.RunStatusCompleted
	require.NoError(t, store.FinishRun(ctx, first, convmodel.Message{}))
	require.EqualValues(t, 5000, peak(ctx))

	second := start("a2")
	step(second, 700)
	second.Status = convmodel.RunStatusFailed
	require.NoError(t, store.FinishRun(ctx, second, convmodel.Message{}))
	require.EqualValues(t, 700, peak(ctx), "the latest finished run wins, not the largest ever")

	third := start("a3")
	step(third, 99000)
	require.EqualValues(t, 700, peak(ctx), "a running run is not counted")
	require.Zero(t, peak(conversationContext("org-b", user)), "another org never reads these steps")
}
