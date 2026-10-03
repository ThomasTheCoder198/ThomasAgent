package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	headerRequestID      = "X-Request-Id"
	headerContentType    = "Content-Type"
	contentTypeJSON      = "application/json; charset=utf-8"
	headerAcceptLanguage = "Accept-Language"
)

type Meta struct {
	RequestID string `json:"requestId"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	TraceID string         `json:"traceId,omitempty"`
}

type dataEnvelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorLogger func(ctx context.Context, err *errors.Error)

type errorLoggerKey struct{}

func withErrorLogger(ctx context.Context, l ErrorLogger) context.Context {
	return context.WithValue(ctx, errorLoggerKey{}, l)
}

func WriteData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, dataEnvelope{Data: data, Meta: Meta{RequestID: RequestIDFrom(r.Context())}})
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := errors.From(err)
	status := appErr.Status()
	body := ErrorBody{
		Code:    string(appErr.Code),
		Message: appErr.LocalizedMessage(errors.LangFromHeader(r.Header.Get(headerAcceptLanguage))),
		TraceID: traceIDFrom(r.Context()),
	}
	if status < http.StatusInternalServerError {
		body.Details = appErr.Details
	} else {
		internal := errors.From(errors.ErrInternalError)
		body.Code = string(internal.Code)
		body.Message = internal.LocalizedMessage(errors.LangFromHeader(r.Header.Get(headerAcceptLanguage)))
		if logErr, ok := r.Context().Value(errorLoggerKey{}).(ErrorLogger); ok && logErr != nil {
			logErr(r.Context(), appErr)
		}
	}
	writeJSON(w, status, errorEnvelope{Error: body})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func traceIDFrom(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}
