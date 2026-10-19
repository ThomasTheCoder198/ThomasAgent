package runstream

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
)

func sampleEvents() []model.Event {
	page := 3
	cost := 0.0042
	return []model.Event{
		model.Start{MessageID: "a1", RunID: uuid.MustParse("11111111-1111-4111-8111-111111111111")},
		model.ReasoningStart{ID: "r1"},
		model.ReasoningDelta{ID: "r1", Delta: "Đang cân nhắc"},
		model.ReasoningEnd{ID: "r1", DurationMS: 120},
		model.ToolInputAvailable{CallID: "c1", Tool: "kb_search", Input: json.RawMessage(`{"query":"giá gói Pro"}`), Line: model.LineKB, Source: model.SourceKB},
		model.ToolOutputAvailable{CallID: "c1", Output: json.RawMessage(`{"results":[{"source":1}]}`), DurationMS: 340},
		model.ToolOutputError{CallID: "c2", ErrorText: "RAGX_UNAVAILABLE", DurationMS: 25},
		model.Status{Label: "Đang tìm tài liệu"},
		model.SourceDocument{SourceID: "S1", Title: "Hợp đồng", FileName: "hd.pdf", Locator: model.CitationLocator{
			FileID: uuid.MustParse("22222222-2222-4222-8222-222222222222"), KBID: uuid.MustParse("33333333-3333-4333-8333-333333333333"),
			FileName: "hd.pdf", FileVersion: 2, KBName: "KB", Section: []string{"Điều 1"}, PageStart: &page, PageEnd: &page,
			BlockIDs: []string{"b1"}, Score: 0.91, Locators: []json.RawMessage{json.RawMessage(`{"kind":"page","page":3}`)},
		}},
		model.TextStart{ID: "t1"},
		model.TextDelta{ID: "t1", Delta: "500k [1]"},
		model.TextEnd{ID: "t1"},
		model.CitationFlags{Flags: []model.CitationFlag{{SourceID: "S1", Claim: "500k", Probability: 0.2}}},
		model.Compaction{SummarizedTurns: 4},
		model.Steer{Text: "ngắn hơn"},
		model.Usage{RunUsage: model.RunUsage{RunID: "run", InputTokens: 10, OutputTokens: 5, CostUSD: &cost, TTFTMS: 80, DurationMS: 900, ContextWindow: 128000}},
		model.Error{Code: "RUN_INTERRUPTED", Message: "Lượt trả lời bị gián đoạn."},
		model.Finish{},
	}
}

func TestCodec_RoundTripsEveryEventType(t *testing.T) {
	events := sampleEvents()
	require.Len(t, events, 18, "one sample per event type; add a sample when model gains an event")
	seen := map[string]bool{}
	for _, event := range events {
		raw, err := EncodeEvent(event)
		require.NoError(t, err)
		decoded, err := DecodeEvent(raw)
		require.NoError(t, err)
		require.Equal(t, event, decoded)
		var envelope struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope))
		require.False(t, seen[envelope.Type], "type names are unique: %s", envelope.Type)
		seen[envelope.Type] = true
	}
}

func TestCodec_RejectsUnknownAndMalformedEntries(t *testing.T) {
	_, err := DecodeEvent([]byte(`{"type":"nope","data":{}}`))
	require.Error(t, err)
	_, err = DecodeEvent([]byte(`not json`))
	require.Error(t, err)
	_, err = DecodeEvent([]byte(`{"type":"text-delta","data":"x"}`))
	require.Error(t, err)
}

func TestIsTerminal(t *testing.T) {
	require.True(t, IsTerminal(model.Finish{}))
	require.True(t, IsTerminal(model.Error{Code: "X"}))
	require.False(t, IsTerminal(model.TextDelta{}))
	require.False(t, IsTerminal(model.Usage{}))
}
