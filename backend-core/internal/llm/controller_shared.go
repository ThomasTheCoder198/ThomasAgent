package llm

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// DeleteResponse is the body of every successful DELETE in this domain.
type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}

// pathUUID reports a malformed id as a field error named after the path parameter.
func pathUUID(c *gin.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, errors.FieldError(name, errors.FieldCodeInvalid)
	}
	return id, nil
}
