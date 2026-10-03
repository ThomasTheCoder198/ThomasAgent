package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  *Meta           `json:"meta"`
	Error *ErrorBody      `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env
}

func newTestRouter(logged *[]*errors.Error) chi.Router {
	r := NewRouter(func(_ context.Context, e *errors.Error) { *logged = append(*logged, e) })
	r.Get("/ok", func(w http.ResponseWriter, req *http.Request) {
		WriteData(w, req, http.StatusOK, map[string]string{"hello": "world"})
	})
	r.Get("/conflict", func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, errors.New(errors.CodeConflict, errors.WithDetails(map[string]any{"field": "name"})))
	})
	r.Get("/boom", func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, context.DeadlineExceeded)
	})
	r.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("secret internal state") })
	return r
}

func do(r http.Handler, method, path, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	req.Header.Set("Accept-Language", lang)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestWriteDataWrapsInEnvelopeWithRequestID(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodGet, "/ok", "vi")
	require.Equal(t, http.StatusOK, rec.Code)
	env := decode(t, rec)
	require.JSONEq(t, `{"hello":"world"}`, string(env.Data))
	require.NotEmpty(t, env.Meta.RequestID)
	require.Equal(t, env.Meta.RequestID, rec.Header().Get(headerRequestID))
}

func TestWriteErrorUsesCatalogAndLanguage(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodGet, "/conflict", "en")
	require.Equal(t, http.StatusConflict, rec.Code)
	env := decode(t, rec)
	require.Equal(t, "CONFLICT", env.Error.Code)
	require.Equal(t, "The request conflicts with the current state.", env.Error.Message)
	require.Equal(t, "name", env.Error.Details["field"])
	require.Empty(t, logged, "4xx errors are not logged as failures")
}

func TestInternalErrorHidesCauseAndIsLoggedOnce(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodGet, "/boom", "vi")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	env := decode(t, rec)
	require.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	require.NotContains(t, rec.Body.String(), "deadline")
	require.Nil(t, env.Error.Details)
	require.Len(t, logged, 1)
}

func TestRouterUnknownRouteUsesEnvelope(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodGet, "/missing", "vi")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "NOT_FOUND", decode(t, rec).Error.Code)
}

func TestRouterWrongMethodUsesEnvelope(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodPost, "/ok", "vi")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "METHOD_NOT_ALLOWED", decode(t, rec).Error.Code)
}

func TestRecoverReturnsInternalEnvelope(t *testing.T) {
	var logged []*errors.Error
	rec := do(newTestRouter(&logged), http.MethodGet, "/panic", "vi")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", decode(t, rec).Error.Code)
	require.NotContains(t, rec.Body.String(), "secret internal state")
	require.NotContains(t, rec.Body.String(), "goroutine")
	require.Len(t, logged, 1)
}

func TestServerErrorSanitizesOverridesAndPreservesTraceID(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID}))
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en")
	rec := httptest.NewRecorder()
	WriteError(rec, req, errors.New(errors.CodeProviderUnavailable,
		errors.WithMessage("secret override"), errors.WithDetails(map[string]any{"secret": "value"})))
	env := decode(t, rec)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	require.Equal(t, "Something went wrong. Please try again.", env.Error.Message)
	require.Equal(t, traceID.String(), env.Error.TraceID)
	require.Nil(t, env.Error.Details)
	require.NotContains(t, rec.Body.String(), "secret")
}

func TestPanicPreservesActiveTraceIDWithNilLogger(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID}))
	r := NewRouter(nil)
	r.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("secret panic") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, traceID.String(), decode(t, rec).Error.TraceID)
	require.NotContains(t, rec.Body.String(), "secret panic")
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestReadyzReportsUnavailableDependency(t *testing.T) {
	var logged []*errors.Error
	r := NewRouter(func(_ context.Context, e *errors.Error) { logged = append(logged, e) })
	MountHealth(r, map[string]Pinger{"postgres": fakePinger{err: context.DeadlineExceeded}})
	rec := do(r, http.MethodGet, "/readyz", "vi")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", decode(t, rec).Error.Code)
	require.Equal(t, errors.CodeProviderUnavailable, logged[0].Code)
	require.Len(t, logged, 1)
}
