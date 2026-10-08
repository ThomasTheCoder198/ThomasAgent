package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type providerOperations interface {
	CreateProvider(context.Context, model.ProviderInput) (model.Provider, error)
	UpdateProvider(context.Context, uuid.UUID, model.ProviderInput) (model.Provider, error)
	DeleteProvider(context.Context, uuid.UUID) error
	ListProviders(context.Context) ([]model.Provider, error)
	TestProvider(context.Context, uuid.UUID) (int, error)
	SyncModels(context.Context, uuid.UUID) (model.SyncResult, error)
}

type ProviderController struct {
	svc          providerOperations
	maxBodyBytes int64
}

func NewProviderController(svc providerOperations, maxBodyBytes int64) *ProviderController {
	return &ProviderController{
		svc:          svc,
		maxBodyBytes: maxBodyBytes,
	}
}

func (h *ProviderController) HandleListProviders(c *gin.Context) {
	r := c.Request
	out, err := h.svc.ListProviders(r.Context())
	httpx.Respond(c, http.StatusOK, dto.FromProviders(out), err)
}

func (h *ProviderController) HandleCreateProvider(c *gin.Context) {
	w, r := c.Writer, c.Request
	var in dto.ProviderRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateProvider(r.Context(), in.ToModel())
	httpx.Respond(c, http.StatusCreated, dto.FromProvider(out), err)
}

func (h *ProviderController) HandleUpdateProvider(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in dto.ProviderRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateProvider(r.Context(), id, in.ToModel())
	httpx.Respond(c, http.StatusOK, dto.FromProvider(out), err)
}

func (h *ProviderController) HandleDeleteProvider(c *gin.Context) {
	r := c.Request
	id, err := pathUUID(c, "providerId")
	if err == nil {
		err = h.svc.DeleteProvider(r.Context(), id)
	}
	httpx.Respond(c, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}

func (h *ProviderController) HandleTestProvider(c *gin.Context) {
	r := c.Request
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	n, err := h.svc.TestProvider(r.Context(), id)
	httpx.Respond(c, http.StatusOK, dto.TestProviderResponse{OK: true, ModelCount: n}, err)
}

func (h *ProviderController) HandleSyncProvider(c *gin.Context) {
	r := c.Request
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.SyncModels(r.Context(), id)
	httpx.Respond(c, http.StatusOK, dto.FromSyncResult(out), err)
}
