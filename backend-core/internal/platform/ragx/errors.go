package ragx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type errorBody struct {
	Error struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

// responseError lets the operator see ragx's 4xx catalog errors unchanged. A failure that is ragx's or core's own
// becomes RAGX_UNAVAILABLE: ragx 5xx, a rejected App token or scope (core misconfiguration, not the operator's
// session), or a code or status the catalog does not know.
func responseError(res *http.Response, limit int64) *errors.AppError {
	var body errorBody
	raw, _ := io.ReadAll(io.LimitReader(res.Body, limit))
	_ = json.Unmarshal(raw, &body)
	code := errors.Code(body.Error.Code)
	cause := fmt.Errorf("ragx answered %d %s", res.StatusCode, code)
	switch {
	case res.StatusCode >= http.StatusInternalServerError,
		res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		return errors.ErrRagxUnavailable.WithCause(cause)
	case code == "" || errors.LookupDefinition(code).HTTPStatus != res.StatusCode:
		return errors.ErrRagxUnavailable.WithCause(cause)
	default:
		return errors.New(code, errors.WithDetails(body.Error.Details))
	}
}
