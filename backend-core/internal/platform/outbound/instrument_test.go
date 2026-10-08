package outbound

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

type recordedCall struct{ target, outcome string }

type fakeRecorder struct{ calls []recordedCall }

func (f *fakeRecorder) ObserveOutbound(target, outcome string, _ time.Duration) {
	f.calls = append(f.calls, recordedCall{target, outcome})
}

func TestInstrument_PropagatesRequestIDOnlyWhenAsked(t *testing.T) {
	const id = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("X-Request-Id"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx := requestid.NewContext(context.Background(), id)
	for _, propagate := range []bool{true, false} {
		client := &http.Client{Transport: Instrument(http.DefaultTransport, TargetRagx, nil, propagate)}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		res, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Empty(t, req.Header.Get("X-Request-Id"), "the caller's request is never mutated")
	}
	require.Equal(t, []string{id, ""}, seen)
}

func TestInstrument_RecordsOutcome(t *testing.T) {
	statuses := []int{http.StatusOK, http.StatusBadGateway}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statuses[0])
		statuses = statuses[1:]
	}))
	defer server.Close()
	recorder := &fakeRecorder{}
	client := &http.Client{Transport: Instrument(http.DefaultTransport, "openrouter", recorder, false)}
	for range 2 {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		res, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
	}
	require.Equal(t, []recordedCall{{"openrouter", metrics.OutcomeSuccess}, {"openrouter", metrics.OutcomeError}}, recorder.calls)
}

func TestInstrument_RemovesSuppliedRequestIDForExternalProviders(t *testing.T) {
	for _, propagate := range []bool{false, true} {
		t.Run(fmt.Sprint(propagate), func(t *testing.T) {
			var received string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				received = req.Header.Get("X-Request-Id")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			ctx := requestid.NewContext(t.Context(), "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			req.Header.Set("X-Request-Id", "req_suppliedid")
			client := &http.Client{Transport: Instrument(http.DefaultTransport, "openrouter", nil, propagate)}
			res, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			require.Empty(t, received)
			require.Equal(t, "req_suppliedid", req.Header.Get("X-Request-Id"))
		})
	}
}

func TestInstrument_RecordsClientErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	recorder := &fakeRecorder{}
	client := &http.Client{Transport: Instrument(http.DefaultTransport, "openrouter", recorder, false)}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, []recordedCall{{"openrouter", metrics.OutcomeError}}, recorder.calls)
}

func TestInstrument_StripsEveryHeaderCaseForExternalProviders(t *testing.T) {
	for _, headers := range []http.Header{
		{"x-request-id": {"req_lowercaseid"}},
		{"X-ReQuEsT-Id": {"req_mixedcaseid"}},
		{"X-Request-Id": {"req_canonicalid"}, "x-request-id": {"req_duplicateid"}},
	} {
		t.Run(fmt.Sprint(headers), func(t *testing.T) {
			var received []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				received = req.Header.Values("X-Request-Id")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			req.Header = headers.Clone()
			client := &http.Client{Transport: Instrument(http.DefaultTransport, "openrouter", nil, false)}
			res, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			require.Empty(t, received)
			require.Equal(t, headers, req.Header)
		})
	}
}
