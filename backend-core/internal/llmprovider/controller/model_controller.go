package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type modelOperations interface {
	CreateModel(context.Context, model.ModelInput) (model.Model, error)
	DeleteModel(context.Context, uuid.UUID) error
	ListModels(context.Context, *uuid.UUID, *model.Capability) ([]model.Model, error)
}

type ModelController struct {
	svc          modelOperations
	maxBodyBytes int64
}

func NewModelController(svc modelOperations, maxBodyBytes int64) *ModelController {
	return &ModelController{
		svc:          svc,
		maxBodyBytes: maxBodyBytes,
	}
}

func (h *ModelController) HandleListModels(c *gin.Context) {
	r := c.Request
	var providerID *uuid.UUID
	if raw := r.URL.Query().Get("providerId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.Fail(c, errors.FieldError("providerId", errors.FieldCodeInvalid))
			return
		}
		providerID = &id
	}
	var capability *model.Capability
	if raw := r.URL.Query().Get("capability"); raw != "" {
		cap := model.Capability(raw)
		if !model.ValidCapability(cap) {
			httpx.Fail(c, errors.FieldError("capability", errors.FieldCodeInvalid))
			return
		}
		capability = &cap
	}
	out, err := h.svc.ListModels(r.Context(), providerID, capability)
	httpx.Respond(c, http.StatusOK, dto.FromModels(out), err)
}

func (h *ModelController) HandleCreateModel(c *gin.Context) {
	w, r := c.Writer, c.Request
	var in dto.ModelRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateModel(r.Context(), in.ToModel())
	httpx.Respond(c, http.StatusCreated, dto.FromModel(out), err)
}

func (h *ModelController) HandleDeleteModel(c *gin.Context) {
	r := c.Request
	id, err := pathUUID(c, "modelId")
	if err == nil {
		err = h.svc.DeleteModel(r.Context(), id)
	}
	httpx.Respond(c, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}
