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

type responseBody struct {
	Data  json.RawMessage `json:"data"`
	Meta  *ResponseMeta   `json:"meta"`
	Error *ErrorDetail    `json:"error"`
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) responseBody {
	t.Helper()
	var response responseBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	return response
}

func newTestRouter(logged *[]*errors.AppError) chi.Router {
	r := NewRouter(func(_ context.Context, e *errors.AppError) { *logged = append(*logged, e) })
	r.Get("/ok", func(w http.ResponseWriter, req *http.Request) {
		WriteSuccess(w, req, http.StatusOK, map[string]string{"hello": "world"})
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

func performRequest(r http.Handler, method, path, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	req.Header.Set("Accept-Language", lang)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestWriteSuccess_ResponseIncludesRequestID(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodGet, "/ok", "vi")
	require.Equal(t, http.StatusOK, rec.Code)
	response := decodeResponse(t, rec)
	require.JSONEq(t, `{"hello":"world"}`, string(response.Data))
	require.NotEmpty(t, response.Meta.RequestID)
	require.Equal(t, response.Meta.RequestID, rec.Header().Get(headerRequestID))
}

func TestWriteError_UsesCatalogAndLanguage(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodGet, "/conflict", "en")
	require.Equal(t, http.StatusConflict, rec.Code)
	response := decodeResponse(t, rec)
	require.Equal(t, "CONFLICT", response.Error.Code)
	require.Equal(t, "The request conflicts with the current state.", response.Error.Message)
	require.Equal(t, "name", response.Error.Details["field"])
	require.Empty(t, logged, "4xx errors are not logged as failures")
}

func TestWriteError_InternalErrorHidesCauseAndIsLoggedOnce(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodGet, "/boom", "vi")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	response := decodeResponse(t, rec)
	require.Equal(t, "INTERNAL_ERROR", response.Error.Code)
	require.NotContains(t, rec.Body.String(), "deadline")
	require.Nil(t, response.Error.Details)
	require.Len(t, logged, 1)
}

func TestRouter_UnknownRouteUsesResponse(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodGet, "/missing", "vi")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "NOT_FOUND", decodeResponse(t, rec).Error.Code)
}

func TestRouter_WrongMethodUsesResponse(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodPost, "/ok", "vi")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "METHOD_NOT_ALLOWED", decodeResponse(t, rec).Error.Code)
}

func TestRecoverPanics_ReturnsInternalResponse(t *testing.T) {
	var logged []*errors.AppError
	rec := performRequest(newTestRouter(&logged), http.MethodGet, "/panic", "vi")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", decodeResponse(t, rec).Error.Code)
	require.NotContains(t, rec.Body.String(), "secret internal state")
	require.NotContains(t, rec.Body.String(), "goroutine")
	require.Len(t, logged, 1)
}

func TestWriteError_ServerErrorSanitizesOverridesAndPreservesTraceID(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID}))
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en")
	rec := httptest.NewRecorder()
	WriteError(rec, req, errors.New(errors.CodeProviderUnavailable,
		errors.WithMessage("secret override"), errors.WithDetails(map[string]any{"secret": "value"})))
	response := decodeResponse(t, rec)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", response.Error.Code)
	require.Equal(t, "Something went wrong. Please try again.", response.Error.Message)
	require.Equal(t, traceID.String(), response.Error.TraceID)
	require.Nil(t, response.Error.Details)
	require.NotContains(t, rec.Body.String(), "secret")
}

func TestRecoverPanics_PreservesActiveTraceIDWithNilLogger(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID}))
	r := NewRouter(nil)
	r.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("secret panic") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, traceID.String(), decodeResponse(t, rec).Error.TraceID)
	require.NotContains(t, rec.Body.String(), "secret panic")
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestMountHealth_ReadinessReportsUnavailableDependency(t *testing.T) {
	var logged []*errors.AppError
	r := NewRouter(func(_ context.Context, e *errors.AppError) { logged = append(logged, e) })
	MountHealth(r, map[string]DependencyPinger{"postgres": fakePinger{err: context.DeadlineExceeded}})
	rec := performRequest(r, http.MethodGet, "/readyz", "vi")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "INTERNAL_ERROR", decodeResponse(t, rec).Error.Code)
	require.Equal(t, errors.CodeProviderUnavailable, logged[0].Code)
	require.Len(t, logged, 1)
}
