package uistream_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/uistream"
)

// B2-6: the stored parts are exactly the parts the AI SDK builds from the same chunks.
func TestMessageBuilder_BuildsTheMessageTheAISDKBuilds(t *testing.T) {
	builder := uistream.NewMessageBuilder(goldenMessageID)
	for _, event := range kbRunEvents() {
		builder.Apply(event)
	}
	parts, err := builder.Parts()
	require.NoError(t, err)

	golden, err := os.ReadFile("testdata/kb-run.message.json")
	require.NoError(t, err)
	want := extractField(t, golden, "parts")
	require.JSONEq(t, want, string(parts))
	require.JSONEq(t, extractField(t, golden, "metadata"), string(builder.Metadata()))
}

func TestMessageBuilder_ClosesOpenTextWhenTheRunStopsMidAnswer(t *testing.T) {
	builder := uistream.NewMessageBuilder("m")
	builder.Apply(model.Start{MessageID: "m", RunID: uuid.New()})
	builder.Apply(model.TextStart{ID: "t1"})
	builder.Apply(model.TextDelta{ID: "t1", Delta: "Một nửa"})

	parts, err := builder.Parts()

	require.NoError(t, err)
	require.JSONEq(t, `[{"type":"text","text":"Một nửa","state":"done"}]`, string(parts))
}
func extractField(t *testing.T, raw []byte, field string) string {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &doc))
	return string(doc[field])
}
