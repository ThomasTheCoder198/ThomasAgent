package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	chatmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	chatworker "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const chatTestOrg = "org-chat"

func TestWorker_ReadyzNeedsTheChatConsumerGroup(t *testing.T) {
	env := startWorkerEnv(t)
	addr := freeAddr(t)
	rt, _ := workerRuntime(t, env, addr, time.Second)
	run := startServeWorker(t, rt, env, []periodic.Job{idleJob("idle")})
	waitHealthy(t, addr)
	ctx := context.Background()

	groups, err := env.redis.XInfoGroups(ctx, convmodel.StreamChatRuns).Result()
	require.NoError(t, err, "the group is created before the worker serves")
	require.Equal(t, chatRunsGroup, groups[0].Name)
	require.Eventually(t, func() bool { status, _ := httpGet(t, "http://"+addr+"/readyz"); return status == http.StatusOK }, workerWait, 20*time.Millisecond)

	require.NoError(t, env.redis.XGroupDestroy(ctx, convmodel.StreamChatRuns, chatRunsGroup).Err())
	require.Eventually(t, func() bool {
		status, _ := httpGet(t, "http://"+addr+"/readyz")
		return status == http.StatusServiceUnavailable
	},
		workerWait, 20*time.Millisecond, "a worker that cannot read chat jobs is not ready")
	status, _ := httpGet(t, "http://"+addr+"/healthz")
	require.Equal(t, http.StatusOK, status)

	// Postgres and Redis stayed reachable the whole time, so the group alone made it unready.
	require.NoError(t, env.redis.XGroupCreateMkStream(ctx, convmodel.StreamChatRuns, chatRunsGroup, "0").Err())
	require.Eventually(t, func() bool { status, _ := httpGet(t, "http://"+addr+"/readyz"); return status == http.StatusOK }, workerWait, 20*time.Millisecond)
	require.NoError(t, run.stop(t))
}

// completingExecutor stands in for the run loop (whose own tests live in chat/service): it streams a start event and
// ends the run completed with a fixed answer through the fenced FinishRun, so the worker path runs without a model.
type completingExecutor struct{ store convservice.ChatStore }

func (e completingExecutor) Execute(ctx context.Context, in chatworker.ExecuteInput) (chatmodel.Event, error) {
	if err := in.Emit(chatmodel.Start{MessageID: in.Run.AssistantMessageID, RunID: in.Run.ID}); err != nil {
		return nil, err
	}
	run := in.Run
	run.Status = convmodel.RunStatusCompleted
	answer := convmodel.Message{ID: run.AssistantMessageID, Role: convmodel.RoleAssistant, Parts: []byte(`[{"type":"text","text":"ok","state":"done"}]`)}
	if err := e.store.FinishRun(ctx, in.Lease, run, answer); err != nil {
		return nil, err
	}
	return chatmodel.Finish{}, nil
}

func newTestChatStore(pool *pgxpool.Pool, rt *coreRuntime) convservice.ChatStore {
	return convservice.NewChatStore(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(), convrepository.NewRunRepo(),
		agentrepository.NewAgentRepo(), rt.cfg.Chat.FinishTimeout)
}

// TestWorker_ARunBegunByTheAPIEndsThroughTheWorker: BeginRun's outbox row is relayed, the chat.runs consumer claims the
// run, and the executor ends it. The run stream carries the events and the terminal event with the id of the request
// that began the run, and the lease is cleared.
func TestWorker_ARunBegunByTheAPIEndsThroughTheWorker(t *testing.T) {
	env := startWorkerEnv(t)
	rt, out := workerRuntime(t, env, freeAddr(t), time.Second)
	ctx := context.Background()
	_, err := env.pool.Exec(ctx, `INSERT INTO organizations (id, tenant_id, name, external_parsing, ragx_status, settings_synced)
		VALUES ($1, $2, $1, false, 'ready', true)`, chatTestOrg, tenant.PlatformID)
	require.NoError(t, err)
	var userID uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($1, 'owner@test.local', 'x') RETURNING id`, tenant.PlatformID).Scan(&userID))
	apiCtx := authmiddleware.WithUser(tenant.WithID(ctx, chatTestOrg), authmodel.User{ID: userID})
	agent, err := agentrepository.NewAgentRepo().InsertAgent(apiCtx, env.pool, agentmodel.Agent{ID: uuid.New(), Name: "A", KBID: uuid.New()})
	require.NoError(t, err)
	store := convservice.NewChatStore(env.pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(), convrepository.NewRunRepo(), agentrepository.NewAgentRepo(), rt.cfg.Chat.FinishTimeout)
	conversation := uuid.New()
	_, _, err = store.EnsureConversation(apiCtx, conversation, agent.ID, userID)
	require.NoError(t, err)
	requestID := requestid.NewRequest()
	message := convmodel.Message{ID: "u1", Role: convmodel.RoleUser, Parts: []byte(`[{"type":"text","text":"hi"}]`)}
	run, _, err := store.BeginRun(requestid.NewContext(apiCtx, requestID), convmodel.BeginRunInput{ConversationID: conversation, AgentID: agent.ID,
		Trigger: convmodel.TriggerSubmitMessage, UserMessage: &message, AssistantMessageID: "a1", Deadline: time.Now().Add(time.Minute)})
	require.NoError(t, err)

	worker := startServeWorker(t, rt, env, []periodic.Job{idleJob("idle")})
	require.Eventually(t, func() bool {
		var status string
		_ = env.pool.QueryRow(ctx, `SELECT status FROM runs WHERE id = $1`, run.ID).Scan(&status)
		return status == string(convmodel.RunStatusCompleted)
	}, workerWait, 20*time.Millisecond, "the worker claimed the run and ended it")

	var code string
	var token *uuid.UUID
	require.NoError(t, env.pool.QueryRow(ctx, `SELECT error_code, lease_token FROM runs WHERE id = $1`, run.ID).Scan(&code, &token))
	require.Empty(t, code)
	require.Nil(t, token, "the lease is cleared")
	entries, err := env.redis.XRange(ctx, runstream.StreamKey(run.ID), "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 2, "start, then the terminal event")
	for _, entry := range entries {
		require.Equal(t, requestID, entry.Values[runstream.FieldRequestID])
	}
	last, err := runstream.DecodeEvent([]byte(entries[1].Values[runstream.FieldEvent].(string)))
	require.NoError(t, err)
	require.IsType(t, chatmodel.Finish{}, last)
	claimed := out.find(t, "chat run claimed")
	require.Len(t, claimed, 1)
	require.Equal(t, requestID, claimed[0]["request_id"], "the worker's logs carry the request id of the chat request")
	require.NoError(t, worker.stop(t))
}
