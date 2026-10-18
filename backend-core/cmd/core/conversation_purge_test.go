package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

type fakeConversationPurger struct{ calls chan struct{} }

func (f fakeConversationPurger) PurgeInactiveConversations(context.Context) (int64, error) {
	f.calls <- struct{}{}
	return 3, nil
}

func TestConversationPurgeJobLogsAndStops(t *testing.T) {
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	app := coreRuntime{cfg: config.Config{Conversation: config.ConversationConfig{RetentionPurgeInterval: time.Millisecond}}, log: log}
	calls := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	go func() { app.purgeInactiveConversations(ctx, fakeConversationPurger{calls}); close(finished) }()
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
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))[0], &record))
	require.Equal(t, conversationsPurgedMessage, record["msg"])
	require.EqualValues(t, 3, record["count"])
	require.Contains(t, record, "duration_ms")
	id, _ := record["request_id"].(string)
	require.True(t, strings.HasPrefix(id, requestid.JobPrefix))
}
