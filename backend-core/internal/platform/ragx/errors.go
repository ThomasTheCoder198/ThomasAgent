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

// catalogCode returns the catalog code of a 4xx body only when that code's catalog status is exactly status. It is
// the one place both the typed calls and the forward decide that a ragx answer is a genuine catalog error.
func catalogCode(status int, parsed errorBody) (errors.Code, bool) {
	code := errors.Code(parsed.Error.Code)
	return code, code != "" && errors.LookupDefinition(code).HTTPStatus == status
}

// responseError lets the operator see ragx's 4xx catalog errors unchanged. A failure that is ragx's or core's own
// becomes RAGX_UNAVAILABLE: ragx 5xx, a rejected App token or scope (core misconfiguration, not the operator's
// session), or a code or status the catalog does not know.
func responseError(res *http.Response, limit int64) *errors.AppError {
	var body errorBody
	raw, _ := io.ReadAll(io.LimitReader(res.Body, limit))
	_ = json.Unmarshal(raw, &body)
	cause := fmt.Errorf("ragx answered %d %s", res.StatusCode, body.Error.Code)
	code, ok := catalogCode(res.StatusCode, body)
	if res.StatusCode >= http.StatusInternalServerError || res.StatusCode == http.StatusUnauthorized ||
		res.StatusCode == http.StatusForbidden || !ok {
		return errors.ErrRagxUnavailable.WithCause(cause)
	}
	return errors.New(code, errors.WithDetails(body.Error.Details))
}
