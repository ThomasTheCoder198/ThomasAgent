package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

func TestChatRepositories_RoundTripAndTenantScope(t *testing.T) {
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	seedReadyOrg(t, pool, "org-b")
	userID := seedUser(t, pool, "owner@test.local")
	ctxA, ctxB := orgContext("org-a"), orgContext("org-b")

	agents := agentrepository.NewAgentRepo()
	override := uuid.New()
	agent := agentmodel.Agent{ID: uuid.New(), Name: "Support", KBID: uuid.New(),
		ModelOverrides: map[llmmodel.Role]uuid.UUID{llmmodel.RoleChatFast: override}}
	stored, err := agents.InsertAgent(ctxA, pool, agent)
	require.NoError(t, err)
	require.Equal(t, override, stored.ModelOverrides[llmmodel.RoleChatFast])
	_, err = agents.InsertAgent(ctxA, pool, agentmodel.Agent{ID: uuid.New(), Name: "Support", KBID: uuid.New()})
	require.Equal(t, errors.CodeConflict, errors.CodeOf(err), "the name is unique per org")
	_, err = agents.InsertAgent(ctxB, pool, agentmodel.Agent{ID: uuid.New(), Name: "Support", KBID: uuid.New()})
	require.NoError(t, err, "another org may reuse the name")
	_, err = agents.GetAgent(ctxB, pool, agent.ID)
	require.Equal(t, errors.CodeAgentNotFound, errors.CodeOf(err), "org B never sees org A's agent")

	conversations := convrepository.NewConversationRepo()
	messages := convrepository.NewMessageRepo()
	runs := convrepository.NewRunRepo()
	conv := convmodel.Conversation{ID: uuid.New(), AgentID: agent.ID, UserID: userID}
	_, err = conversations.InsertConversation(ctxA, pool, conv)
	require.NoError(t, err)
	_, err = conversations.GetConversation(ctxB, pool, conv.ID)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))

	parts := json.RawMessage(`[{"type":"text","text":"Xin chào"}]`)
	require.NoError(t, messages.InsertMessage(ctxA, pool, conv.ID, convmodel.Message{ID: "m1", Role: convmodel.RoleUser, Parts: parts}))
	require.NoError(t, messages.InsertMessage(ctxA, pool, conv.ID, convmodel.Message{ID: "m2", Role: convmodel.RoleAssistant, Parts: parts}))
	err = messages.InsertMessage(ctxA, pool, conv.ID, convmodel.Message{ID: "m1", Role: convmodel.RoleUser, Parts: parts})
	require.Equal(t, errors.CodeConflict, errors.CodeOf(err), "a repeated client id in one conversation is a conflict")
	list, err := messages.ListLatest(ctxA, pool, conv.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, int64(1), list[0].Seq)
	require.Equal(t, "m2", list[1].ID)
	require.JSONEq(t, string(parts), string(list[0].Parts))
	none, err := messages.ListLatest(ctxB, pool, conv.ID, 10)
	require.NoError(t, err)
	require.Empty(t, none)

	run := convmodel.Run{ID: uuid.New(), ConversationID: conv.ID, AssistantMessageID: "m3", Status: convmodel.RunStatusRunning,
		DeadlineAt: time.Now().Add(time.Minute)}
	require.NoError(t, runs.InsertRun(ctxA, pool, run))
	err = runs.InsertRun(ctxA, pool, convmodel.Run{ID: uuid.New(), ConversationID: conv.ID, AssistantMessageID: "m4",
		Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)})
	require.Equal(t, errors.CodeRunAlreadyActive, errors.CodeOf(err))
	require.NoError(t, runs.InsertStep(ctxA, pool, convmodel.RunStep{RunID: run.ID, Step: 1, Kind: convmodel.StepKindTool,
		Name: "kb_search", Outcome: convmodel.StepOutcomeSuccess, DurationMS: 12, StartedAt: time.Now()}))
	_, err = runs.GetRun(ctxB, pool, run.ID)
	require.Equal(t, errors.CodeRunNotActive, errors.CodeOf(err), "another org's run is reported as not active, never as found")
	run.Status = convmodel.RunStatusCompleted
	require.NoError(t, runs.FinishRun(ctxA, pool, run))
	require.Equal(t, errors.CodeRunNotActive, errors.CodeOf(runs.FinishRun(ctxA, pool, run)), "a run finishes once")
}

// chatFixture is two ready orgs; org A owns an agent, a conversation with two messages and a running run.
type chatFixture struct {
	pool           *pgxpool.Pool
	userID         uuid.UUID
	ctxA, ctxB     context.Context
	agentA, agentB uuid.UUID
	convA, convB   uuid.UUID
	runA           uuid.UUID
	conversations  convrepository.ConversationRepo
	messages       convrepository.MessageRepo
	runs           convrepository.RunRepo
}

func newChatFixture(t *testing.T) chatFixture {
	t.Helper()
	pool := pgtest.Start(t)
	seedReadyOrg(t, pool, "org-a")
	seedReadyOrg(t, pool, "org-b")
	f := chatFixture{pool: pool, userID: seedUser(t, pool, "owner@test.local"), ctxA: orgContext("org-a"), ctxB: orgContext("org-b"),
		agentA: uuid.New(), agentB: uuid.New(), convA: uuid.New(), convB: uuid.New(), runA: uuid.New(),
		conversations: convrepository.NewConversationRepo(), messages: convrepository.NewMessageRepo(), runs: convrepository.NewRunRepo()}
	agents := agentrepository.NewAgentRepo()
	_, err := agents.InsertAgent(f.ctxA, pool, agentmodel.Agent{ID: f.agentA, Name: "A", KBID: uuid.New()})
	require.NoError(t, err)
	_, err = agents.InsertAgent(f.ctxB, pool, agentmodel.Agent{ID: f.agentB, Name: "B", KBID: uuid.New()})
	require.NoError(t, err)
	_, err = f.conversations.InsertConversation(f.ctxA, pool, convmodel.Conversation{ID: f.convA, AgentID: f.agentA, UserID: f.userID})
	require.NoError(t, err)
	_, err = f.conversations.InsertConversation(f.ctxB, pool, convmodel.Conversation{ID: f.convB, AgentID: f.agentB, UserID: f.userID})
	require.NoError(t, err)
	parts := json.RawMessage(`[{"type":"text","text":"hi"}]`)
	require.NoError(t, f.messages.InsertMessage(f.ctxA, pool, f.convA, convmodel.Message{ID: "a1", Role: convmodel.RoleUser, Parts: parts}))
	require.NoError(t, f.messages.InsertMessage(f.ctxA, pool, f.convA, convmodel.Message{ID: "a2", Role: convmodel.RoleAssistant, Parts: parts}))
	require.NoError(t, f.runs.InsertRun(f.ctxA, pool, convmodel.Run{ID: f.runA, ConversationID: f.convA, AssistantMessageID: "a3",
		Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}))
	return f
}

// requireCode fails the test, instead of panicking in CodeOf, when the repository accepted a write it should refuse.
func requireCode(t *testing.T, want errors.Code, err error, msg string) {
	t.Helper()
	require.Error(t, err, msg)
	require.Equal(t, want, errors.CodeOf(err), msg)
}

func TestChatRepositories_RejectCrossTenantInserts(t *testing.T) {
	f := newChatFixture(t)
	parts := json.RawMessage(`[{"type":"text","text":"intrusion"}]`)

	t.Run("conversation of another org's agent", func(t *testing.T) {
		_, err := f.conversations.InsertConversation(f.ctxB, f.pool, convmodel.Conversation{ID: uuid.New(), AgentID: f.agentA, UserID: f.userID})
		requireCode(t, errors.CodeAgentNotFound, err, "a conversation cannot hang off another org's agent")
	})

	t.Run("message in another org's conversation", func(t *testing.T) {
		err := f.messages.InsertMessage(f.ctxB, f.pool, f.convA, convmodel.Message{ID: "intruder", Role: convmodel.RoleUser, Parts: parts})
		requireCode(t, errors.CodeConversationNotFound, err, "a message cannot join another org's conversation")
		victim, err := f.messages.ListAll(f.ctxA, f.pool, f.convA)
		require.NoError(t, err)
		require.Len(t, victim, 2, "the victim's messages are unchanged")
		require.NoError(t, f.messages.InsertMessage(f.ctxA, f.pool, f.convA, convmodel.Message{ID: "a3", Role: convmodel.RoleUser, Parts: parts}))
		next, found, err := f.messages.Last(f.ctxA, f.pool, f.convA)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, int64(3), next.Seq, "the owner's next seq is still contiguous")
	})

	t.Run("run on another org's conversation", func(t *testing.T) {
		err := f.runs.InsertRun(f.ctxB, f.pool, convmodel.Run{ID: uuid.New(), ConversationID: f.convA, AssistantMessageID: "x",
			Status: convmodel.RunStatusCompleted, DeadlineAt: time.Now().Add(time.Minute)})
		requireCode(t, errors.CodeConversationNotFound, err, "a run cannot attach to another org's conversation")
	})

	t.Run("step on another org's run", func(t *testing.T) {
		err := f.runs.InsertStep(f.ctxB, f.pool, convmodel.RunStep{RunID: f.runA, Step: 1, Kind: convmodel.StepKindTool,
			Outcome: convmodel.StepOutcomeSuccess, StartedAt: time.Now()})
		requireCode(t, errors.CodeRunNotActive, err, "a step cannot attach to another org's run")
		var steps int
		require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM run_steps WHERE run_id = $1`, f.runA).Scan(&steps))
		require.Zero(t, steps)
	})
}

// The repositories prove the parent first; the composite foreign keys make the database refuse the same writes
// even if a later query forgets to.
func TestChatRepositories_DatabaseRefusesMismatchedTenantParent(t *testing.T) {
	f := newChatFixture(t)
	ctx := context.Background()
	cases := map[string]struct {
		sql  string
		args []any
	}{
		"conversation of another org's agent": {`INSERT INTO conversations (id, tenant_id, agent_id, user_id) VALUES ($1, 'org-b', $2, $3)`,
			[]any{uuid.New(), f.agentA, f.userID}},
		"message of another org's conversation": {`INSERT INTO messages (conversation_id, id, tenant_id, seq, role, parts) VALUES ($1, 'x', 'org-b', 99, 'user', '[]')`,
			[]any{f.convA}},
		"run of another org's conversation": {`INSERT INTO runs (id, tenant_id, conversation_id, assistant_message_id, status, deadline_at) VALUES ($1, 'org-b', $2, 'x', 'completed', now())`,
			[]any{uuid.New(), f.convA}},
		"step of another org's run": {`INSERT INTO run_steps (run_id, step, tenant_id, kind, outcome, duration_ms, started_at) VALUES ($1, 1, 'org-b', 'tool', 'success', 1, now())`,
			[]any{f.runA}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.pool.Exec(ctx, c.sql, c.args...)
			require.True(t, postgres.IsForeignKeyViolation(err), "got %v", err)
		})
	}
}

func TestChatRepositories_ListRangeBoundaries(t *testing.T) {
	f := newChatFixture(t)
	parts := json.RawMessage(`[{"type":"text","text":"x"}]`)
	for _, id := range []string{"a3", "a4", "a5"} {
		require.NoError(t, f.messages.InsertMessage(f.ctxA, f.pool, f.convA, convmodel.Message{ID: id, Role: convmodel.RoleUser, Parts: parts}))
	}
	ids := func(list []convmodel.Message) []string {
		out := []string{}
		for _, m := range list {
			out = append(out, m.ID)
		}
		return out
	}

	list, err := f.messages.ListRange(f.ctxA, f.pool, f.convA, 1, 4, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"a2", "a3", "a4"}, ids(list), "afterSeq is exclusive, throughSeq is inclusive, oldest first")

	list, err = f.messages.ListRange(f.ctxA, f.pool, f.convA, 0, 5, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"a1", "a2"}, ids(list), "limit keeps the oldest rows of the range")

	list, err = f.messages.ListRange(f.ctxA, f.pool, f.convA, 5, 5, 10)
	require.NoError(t, err)
	require.Empty(t, list, "an empty range is empty")

	list, err = f.messages.ListRange(f.ctxB, f.pool, f.convA, 0, 5, 10)
	require.NoError(t, err)
	require.Empty(t, list, "another org reads nothing")
}
