package model

import (
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// ErrorEvent turns a failure into the in-stream error, masked like an HTTP 5xx body (rule 4).
func ErrorEvent(err error, lang errors.Lang) Error {
	app := errors.ToAppError(err)
	if app.HTTPStatus() >= http.StatusInternalServerError && !publicServerError(app.Code) {
		app = errors.New(errors.CodeInternalError)
	}
	return Error{Code: string(app.Code), Message: app.LocalizedMessage(lang)}
}

// publicServerError lists the 5xx codes a client may see: they say what to do next (retry, reload) and expose nothing
// internal. RUN_INTERRUPTED and RUN_STREAM_INCOMPLETE are the worker/relay endings of a run (amendment W4, W5).
func publicServerError(code errors.Code) bool {
	switch code {
	case errors.CodeProviderUnavailable, errors.CodeRagxUnavailable, errors.CodeDependencyTimeout,
		errors.CodeRunDeadlineExceeded, errors.CodeRunInterrupted, errors.CodeRunStreamIncomplete:
		return true
	default:
		return false
	}
}
