package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

type recordedHTTP struct {
	method, route, requestID string
	status                   int
}

type fakeHTTPMetrics struct{ calls []recordedHTTP }

func (f *fakeHTTPMetrics) ObserveHTTP(method, route string, status int, _ time.Duration, requestID string) {
	f.calls = append(f.calls, recordedHTTP{method: method, route: route, requestID: requestID, status: status})
}

func TestRecordMetrics_UsesRoutePatternStatusAndRequestID(t *testing.T) {
	fake := &fakeHTTPMetrics{}
	r := NewRouter(nil, RecordMetrics(fake))
	r.Get("/api/v1/providers/{id}", func(w http.ResponseWriter, req *http.Request) {
		WriteSuccess(w, req, http.StatusOK, nil)
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/providers/abc", nil)
	req.Header.Set(requestid.Header, "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
	r.ServeHTTP(httptest.NewRecorder(), req)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nope", nil))
	require.Len(t, fake.calls, 2)
	require.Equal(t, recordedHTTP{method: "GET", route: "/api/v1/providers/{id}", requestID: "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21", status: 200}, fake.calls[0])
	require.Equal(t, "unmatched", fake.calls[1].route)
	require.Equal(t, http.StatusNotFound, fake.calls[1].status)
}

func TestRecordMetrics_NilRecorderIsPassThrough(t *testing.T) {
	r := NewRouter(nil, RecordMetrics(nil))
	r.Get("/ok", func(w http.ResponseWriter, req *http.Request) { WriteSuccess(w, req, http.StatusOK, nil) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ok", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}
