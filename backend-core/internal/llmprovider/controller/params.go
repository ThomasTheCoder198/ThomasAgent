package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func pathUUID(c *gin.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, errors.FieldError(name, errors.FieldCodeInvalid)
	}
	return id, nil
}
