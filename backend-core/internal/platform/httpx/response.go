package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const (
	headerRequestID      = "X-Request-Id"
	headerContentType    = "Content-Type"
	contentTypeJSON      = "application/json; charset=utf-8"
	headerAcceptLanguage = "Accept-Language"
)

type ResponseMeta struct {
	RequestID string `json:"requestId"`
}

type ErrorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	TraceID string         `json:"traceId,omitempty"`
}

type successResponse struct {
	Data any          `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

type errorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorLogger func(ctx context.Context, err *errors.AppError)

type errorLoggerKey struct{}

func withErrorLogger(ctx context.Context, l ErrorLogger) context.Context {
	return context.WithValue(ctx, errorLoggerKey{}, l)
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, successResponse{Data: data, Meta: ResponseMeta{RequestID: logging.RequestIDFromContext(r.Context())}})
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := errors.ToAppError(err)
	if ResponseCommitted(w) {
		// The client already holds part of a response; the failure can only be recorded.
		logBoundaryError(r, appErr)
		return
	}
	status := appErr.HTTPStatus()
	body := ErrorDetail{
		Code:    string(appErr.Code),
		Message: appErr.LocalizedMessage(errors.LangFromHeader(r.Header.Get(headerAcceptLanguage))),
		TraceID: traceIDFromContext(r.Context()),
	}
	if status < http.StatusInternalServerError {
		body.Details = appErr.Details
	} else {
		internal := errors.ToAppError(errors.ErrInternalError)
		body.Code = string(internal.Code)
		body.Message = internal.LocalizedMessage(errors.LangFromHeader(r.Header.Get(headerAcceptLanguage)))
		logBoundaryError(r, appErr)
	}
	writeJSON(w, status, errorResponse{Error: body})
}

func logBoundaryError(r *http.Request, appErr *errors.AppError) {
	if errorLogger, ok := r.Context().Value(errorLoggerKey{}).(ErrorLogger); ok && errorLogger != nil {
		errorLogger(r.Context(), appErr)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func traceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}
