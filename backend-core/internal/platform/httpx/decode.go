package httpx

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"mime"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const fieldBody = "body"

var errTrailingJSON = stderrors.New("unexpected data after JSON value")

// The decoder error is kept only as the cause (never serialized), so request content cannot reach a response.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	if err != nil || mediaType != mediaTypeJSON {
		return errors.ErrUnsupportedMediaType
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
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
