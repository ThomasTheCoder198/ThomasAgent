package genkitxtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const (
	decisionInputTokens  = 5
	decisionOutputTokens = 1
)

// DecisionsServer answers System One requests (`POST /api/alpha/decisions` for OpenRouter) with scripted answers:
// one map per request, keyed by question id.
type DecisionsServer struct {
	URL      string
	mu       sync.Mutex
	answers  []map[string]any
	requests []Request
	status   int
}

func NewDecisionsServer(t testing.TB, answers ...map[string]any) *DecisionsServer {
	t.Helper()
	s := &DecisionsServer{answers: answers}
	server := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(server.Close)
	s.URL = server.URL
	return s
}

func (s *DecisionsServer) Push(answers ...map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers = append(s.answers, answers...)
}

// Fail makes every later request answer status.
func (s *DecisionsServer) Fail(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *DecisionsServer) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *DecisionsServer) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	status := s.status
	var answers map[string]any
	if status == 0 && len(s.answers) > 0 {
		answers, s.answers = s.answers[0], s.answers[1:]
	}
	s.mu.Unlock()
	if status != 0 || answers == nil {
		if status == 0 {
			status = http.StatusInternalServerError
		}
		writeError(w, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": body["model"], "answers": answers,
		"usage": map[string]any{"input_tokens": decisionInputTokens, "output_tokens": decisionOutputTokens},
	})
}
