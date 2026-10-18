// Package genkitxtest serves scripted OpenAI-compatible and System One responses, so Genkit runs end to end in
// tests without a provider. It is test support: nothing in production imports it.
package genkitxtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	finishStop      = "stop"
	finishToolCalls = "tool_calls"
	sseDone         = "data: [DONE]\n\n"
	chunkObject     = "chat.completion.chunk"
	completionObj   = "chat.completion"
	fixtureID       = "fixture"
)

// ToolCall is one function call the scripted model makes.
type ToolCall struct{ ID, Name, Arguments string }

// Turn is one scripted model answer. Each request takes the next turn. Status answers that HTTP status instead,
// and Hold waits first, giving up as soon as the client does.
type Turn struct {
	Reasoning                 string
	Text                      string
	ToolCalls                 []ToolCall
	InputTokens, OutputTokens int
	Status                    int
	Hold                      time.Duration
}

// Request is what the fixture received.
type Request struct {
	Path   string
	Header http.Header
	Body   map[string]any
}

type OpenAIServer struct {
	URL      string
	mu       sync.Mutex
	turns    []Turn
	requests []Request
}

func NewOpenAIServer(t testing.TB, turns ...Turn) *OpenAIServer {
	t.Helper()
	s := &OpenAIServer{turns: turns}
	server := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(server.Close)
	s.URL = server.URL
	return s
}

func (s *OpenAIServer) Push(turns ...Turn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns = append(s.turns, turns...)
}

func (s *OpenAIServer) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *OpenAIServer) next(r *http.Request, body map[string]any) (Turn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	if len(s.turns) == 0 {
		return Turn{}, false
	}
	turn := s.turns[0]
	s.turns = s.turns[1:]
	return turn, true
}

func (s *OpenAIServer) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	turn, ok := s.next(r, body)
	if !ok {
		writeError(w, http.StatusInternalServerError)
		return
	}
	if turn.Hold > 0 {
		select {
		case <-time.After(turn.Hold):
		case <-r.Context().Done():
			return
		}
	}
	if turn.Status != 0 {
		writeError(w, turn.Status)
		return
	}
	model, _ := body["model"].(string)
	if stream, _ := body["stream"].(bool); stream {
		writeStream(w, model, turn)
		return
	}
	writeCompletion(w, model, turn)
}

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": "fixture error", "type": "fixture", "code": status},
	})
}

func finishReason(turn Turn) string {
	if len(turn.ToolCalls) > 0 {
		return finishToolCalls
	}
	return finishStop
}

func usage(turn Turn) map[string]any {
	return map[string]any{
		"prompt_tokens": turn.InputTokens, "completion_tokens": turn.OutputTokens,
		"total_tokens": turn.InputTokens + turn.OutputTokens,
	}
}

func toolCalls(turn Turn) []map[string]any {
	calls := make([]map[string]any, 0, len(turn.ToolCalls))
	for i, call := range turn.ToolCalls {
		calls = append(calls, map[string]any{
			"index": i, "id": call.ID, "type": "function",
			"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
		})
	}
	return calls
}

func writeCompletion(w http.ResponseWriter, model string, turn Turn) {
	message := map[string]any{"role": "assistant", "content": turn.Text}
	if len(turn.ToolCalls) > 0 {
		message["tool_calls"] = toolCalls(turn)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": fixtureID, "object": completionObj, "created": 1, "model": model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason(turn)}},
		"usage":   usage(turn),
	})
}

func writeStream(w http.ResponseWriter, model string, turn Turn) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	send := func(v any) {
		raw, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": fixtureID, "object": chunkObject, "created": 1, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	if turn.Reasoning != "" {
		send(chunk(map[string]any{"role": "assistant", "content": "", "reasoning": turn.Reasoning}, nil))
	}
	for _, word := range strings.SplitAfter(turn.Text, " ") {
		if word != "" {
			send(chunk(map[string]any{"content": word}, nil))
		}
	}
	if calls := toolCalls(turn); len(calls) > 0 {
		send(chunk(map[string]any{"tool_calls": calls}, nil))
	}
	send(chunk(map[string]any{}, finishReason(turn)))
	send(map[string]any{
		"id": fixtureID, "object": chunkObject, "created": 1, "model": model, "choices": []any{}, "usage": usage(turn),
	})
	_, _ = io.WriteString(w, sseDone)
	if flusher != nil {
		flusher.Flush()
	}
}
