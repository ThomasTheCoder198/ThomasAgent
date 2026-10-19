package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/periodic"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

type fakeConversationPurger struct{ calls chan struct{} }

func (f fakeConversationPurger) PurgeInactiveConversations(context.Context) (int64, error) {
	f.calls <- struct{}{}
	return 3, nil
}

func TestConversationPurgeJobLogsAndStops(t *testing.T) {
	output := &syncBuffer{}
	log, err := logging.NewLogger(output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	app := coreRuntime{cfg: config.Config{Conversation: config.ConversationConfig{RetentionPurgeInterval: time.Millisecond}, ConversationRetentionPurgeTimeout: time.Second}, log: log}
	calls := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	runner := periodic.Runner{Redis: startWorkerTestRedis(t), Log: log}
	go func() {
		_ = runner.Run(ctx, []periodic.Job{app.conversationPurgeJob(fakeConversationPurger{calls})})
		close(finished)
	}()
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("purge job did not run")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("purge job did not stop")
	}
	records := output.find(t, "periodic job completed")
	require.NotEmpty(t, records)
	record := records[0]
	require.Equal(t, conversationPurgeJob, record["job"])
	require.EqualValues(t, 3, record[logging.FieldCount])
	require.Contains(t, record, logging.FieldDurationMS)
	id, _ := record["request_id"].(string)
	require.True(t, strings.HasPrefix(id, requestid.JobPrefix), "got request_id %q", id)
}
