package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	testRunDeadline   = time.Minute
	testRunLease      = 30 * time.Second
	testFinishTimeout = 5 * time.Second
	testWorkerOwner   = "worker-test"
)

// withRequestID gives ctx the id a chat request would have after the request-id middleware.
func withRequestID(ctx context.Context) (context.Context, string) {
	id := requestid.NewRequest()
	return requestid.NewContext(ctx, id), id
}

func submitInput(conv, agent uuid.UUID, messageID, text, assistantID string) convmodel.BeginRunInput {
	msg := userText(messageID, text)
	return convmodel.BeginRunInput{ConversationID: conv, AgentID: agent, Trigger: convmodel.TriggerSubmitMessage,
		UserMessage: &msg, AssistantMessageID: assistantID, Deadline: time.Now().Add(testRunDeadline)}
}

func regenerateInput(conv, agent uuid.UUID, messageID, assistantID string) convmodel.BeginRunInput {
	return convmodel.BeginRunInput{ConversationID: conv, AgentID: agent, Trigger: convmodel.TriggerRegenerateMessage,
		RegenerateMessageID: messageID, AssistantMessageID: assistantID, Deadline: time.Now().Add(testRunDeadline)}
}

// beginSubmit appends a user message and starts a run, the way the chat endpoint will.
func beginSubmit(t *testing.T, store convservice.ChatStore, ctx context.Context, conv, agent uuid.UUID, messageID, assistantID string) (convmodel.Run, string) {
	t.Helper()
	run, outboxID, err := store.BeginRun(ctx, submitInput(conv, agent, messageID, "Giá gói Pro?", assistantID))
	require.NoError(t, err)
	require.NotEmpty(t, outboxID)
	return run, outboxID
}

// claim takes the lease of a run the way the worker does.
func claim(t *testing.T, store convservice.ChatStore, ctx context.Context, run convmodel.Run) convmodel.Lease {
	t.Helper()
	claimed, lease, result, err := store.ClaimRun(ctx, run.ID, testWorkerOwner, testRunLease)
	require.NoError(t, err)
	require.Equal(t, convmodel.ClaimResultClaimed, result)
	require.Equal(t, run.ID, claimed.ID)
	require.Equal(t, run.ID, lease.RunID)
	require.NotEqual(t, uuid.Nil, lease.Token)
	return lease
}

// appendUserMessage stores a user message without a run: the conversation read tests only need messages.
func appendUserMessage(t *testing.T, pool *pgxpool.Pool, ctx context.Context, conversationID uuid.UUID, msg convmodel.Message) {
	t.Helper()
	msg.Role = convmodel.RoleUser
	require.NoError(t, convrepository.NewMessageRepo().InsertMessage(ctx, pool, conversationID, msg))
	require.NoError(t, convrepository.NewConversationRepo().Touch(ctx, pool, conversationID))
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(context.Background(), query, args...).Scan(&count))
	return count
}
