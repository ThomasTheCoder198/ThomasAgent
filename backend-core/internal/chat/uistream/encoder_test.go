package uistream_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/uistream"
)

const (
	goldenRunID     = "11111111-2222-4333-8444-555555555555"
	goldenMessageID = "msg-golden"
	// goldenSearchOutput is SEARCH_OUTPUT of the capture script: ragx data after run-wide renumbering.
	goldenSearchOutput = `{"results":[{"source":1,"chunkId":"c-1","fileId":"2b1e7c3a-0000-4000-8000-000000000002",` +
		`"fileName":"policy.pdf","nodeId":"9b2f0c1e-0000-4000-8000-000000000001","section":["Chính sách","Hoàn tiền"],` +
		`"pageStart":3,"pageEnd":3,"score":0.91,"blocks":[{"label":"[1.1]","blockId":"b-1",` +
		`"text":"Khách được hoàn tiền trong 7 ngày.","locator":{"kind":"pdf_bbox","page":3,"bbox":[10,20,300,80]}}]}],` +
		`"insufficientEvidence":false}`
)

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }

// kbRunEvents is the event sequence a run emits for the kb-run golden (same values as the capture script).
func kbRunEvents() []model.Event {
	runID := uuid.MustParse(goldenRunID)
	return []model.Event{
		model.Start{MessageID: goldenMessageID, RunID: runID},
		model.Compaction{SummarizedTurns: 6},
		model.ReasoningStart{ID: "r1"},
		model.ReasoningDelta{ID: "r1", Delta: "Cần tìm "},
		model.ReasoningDelta{ID: "r1", Delta: "chính sách."},
		model.ReasoningEnd{ID: "r1", DurationMS: 900},
		model.ToolInputAvailable{CallID: "call-1", Tool: "kb_search", Input: json.RawMessage(`{"query":"chính sách hoàn tiền"}`),
			Line: model.LineKB, Source: model.SourceKB},
		model.Status{Label: "Đang tìm trong KB…"},
		model.ToolOutputAvailable{CallID: "call-1", Output: json.RawMessage(goldenSearchOutput), DurationMS: 640},
		model.ToolInputAvailable{CallID: "call-2", Tool: "kb_read_section",
			Input: json.RawMessage(`{"nodeId":"9b2f0c1e-0000-4000-8000-000000000001"}`), Line: model.LineKB, Source: model.SourceKB},
		model.ToolOutputError{CallID: "call-2", ErrorText: "DEPENDENCY_TIMEOUT", DurationMS: 20000},
		model.Steer{Text: "Chỉ cần bảng giá"},
		model.SourceDocument{SourceID: "S1", Title: "Chính sách › Hoàn tiền", FileName: "policy.pdf", Locator: model.CitationLocator{
			FileID: uuid.MustParse("2b1e7c3a-0000-4000-8000-000000000002"), KBID: uuid.MustParse("3c2f8d4b-0000-4000-8000-000000000003"),
			FileName: "policy.pdf", KBName: "Chính sách", Section: []string{"Chính sách", "Hoàn tiền"},
			PageStart: intPtr(3), PageEnd: intPtr(3), BlockIDs: []string{"b-1"}, Score: 0.91,
			Locators: []json.RawMessage{json.RawMessage(`{"kind":"pdf_bbox","page":3,"bbox":[10,20,300,80]}`)},
		}},
		model.TextStart{ID: "t1"},
		model.TextDelta{ID: "t1", Delta: "Khách được hoàn tiền "},
		model.TextDelta{ID: "t1", Delta: "trong 7 ngày [1]."},
		model.TextEnd{ID: "t1"},
		model.CitationFlags{Flags: []model.CitationFlag{{SourceID: "S1", Claim: "Khách được hoàn tiền trong 7 ngày", Probability: 0.2}}},
		model.Usage{RunUsage: model.RunUsage{RunID: goldenRunID, InputTokens: 1200, OutputTokens: 80, CostUSD: floatPtr(0.0012),
			TTFTMS: 700, DurationMS: 3100, ContextWindow: 128000}},
		model.Finish{},
	}
}

// goldenFrames reads the data: payloads of an SSE golden, in order.
func goldenFrames(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var frames []string
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for scanner.Scan() {
		if payload, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			frames = append(frames, payload)
		}
	}
	return frames
}

func encodeAll(t *testing.T, events []model.Event) []string {
	t.Helper()
	encoder := uistream.NewEncoder()
	var frames []string
	for _, event := range events {
		encoded, err := encoder.Encode(event)
		require.NoError(t, err)
		for _, frame := range encoded {
			require.NotContains(t, string(frame), "\n", "one SSE data line per chunk")
			frames = append(frames, string(frame))
		}
	}
	return append(frames, string(uistream.DoneFrame))
}

func requireSameFrames(t *testing.T, want, got []string) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		if want[i] == string(uistream.DoneFrame) {
			require.Equal(t, want[i], got[i])
			continue
		}
		require.JSONEq(t, want[i], got[i], "frame %d", i)
	}
}

func TestEncoder_MatchesTheAISDKGoldenForAKBRun(t *testing.T) {
	requireSameFrames(t, goldenFrames(t, "kb-run.sse"), encodeAll(t, kbRunEvents()))
}

// B2-7: the error part carries the catalog code only; the web localizes it.
func TestEncoder_ErrorPartCarriesTheCatalogCodeOnly(t *testing.T) {
	events := []model.Event{
		model.Start{MessageID: goldenMessageID, RunID: uuid.MustParse(goldenRunID)},
		model.TextStart{ID: "t1"},
		model.TextDelta{ID: "t1", Delta: "Đang trả lời"},
		model.Error{Code: "PROVIDER_UNAVAILABLE", Message: "Dịch vụ bên ngoài đang không khả dụng."},
	}
	requireSameFrames(t, goldenFrames(t, "error.sse"), encodeAll(t, events))
}

func TestEncoder_StatusIDsAreStablePerRun(t *testing.T) {
	encoder := uistream.NewEncoder()
	_, err := encoder.Encode(model.Start{MessageID: "m", RunID: uuid.MustParse(goldenRunID)})
	require.NoError(t, err)
	first, err := encoder.Encode(model.Status{Label: "a"})
	require.NoError(t, err)
	second, err := encoder.Encode(model.Status{Label: "b"})
	require.NoError(t, err)
	require.Contains(t, string(first[0]), goldenRunID+"-s1")
	require.Contains(t, string(second[0]), goldenRunID+"-s2")
}

func TestEncoder_ToolOutputForAnUnknownCallIsAnError(t *testing.T) {
	encoder := uistream.NewEncoder()
	_, err := encoder.Encode(model.ToolOutputAvailable{CallID: "never-started"})
	require.Error(t, err)
}
