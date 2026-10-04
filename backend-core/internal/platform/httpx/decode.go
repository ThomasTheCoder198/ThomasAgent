package httpx

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const fieldBody = "body"

var errTrailingJSON = stderrors.New("unexpected data after JSON value")

// DecodeJSON reads exactly one JSON value into dst. Bodies above maxBytes become PAYLOAD_TOO_LARGE; anything else
// that is not a single valid value becomes VALIDATION_FAILED on field "body". The decoder error is kept only as the
// cause (never serialized), so request content cannot reach a response.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	if err := decoder.Decode(dst); err != nil {
		return decodeFailure(err)
	}
	switch err := decoder.Decode(&struct{}{}); {
	case stderrors.Is(err, io.EOF):
		return nil
	case err == nil:
		return decodeFailure(errTrailingJSON)
	default:
		return decodeFailure(err)
	}
}

func decodeFailure(err error) error {
	var tooLarge *http.MaxBytesError
	if stderrors.As(err, &tooLarge) {
		return errors.ErrPayloadTooLarge.WithCause(err)
	}
	return errors.FieldError(fieldBody, errors.FieldCodeInvalid).WithCause(err)
}
