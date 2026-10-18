package model

import (
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// ErrorEvent turns a failure into the in-stream error, masked like an HTTP 5xx body (rule 4).
func ErrorEvent(err error, lang errors.Lang) Error {
	app := errors.ToAppError(err)
	if app.HTTPStatus() >= http.StatusInternalServerError && app.Code != errors.CodeProviderUnavailable &&
		app.Code != errors.CodeRagxUnavailable && app.Code != errors.CodeDependencyTimeout &&
		app.Code != errors.CodeRunDeadlineExceeded {
		app = errors.New(errors.CodeInternalError)
	}
	return Error{Code: string(app.Code), Message: app.LocalizedMessage(lang)}
}
