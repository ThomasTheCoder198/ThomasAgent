package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/service"
)

type Handler struct {
	svc          service.RegistryService
	maxBodyBytes int64
}

func NewRegistryHandler(svc service.RegistryService, maxBodyBytes int64) *Handler {
	return &Handler{svc: svc, maxBodyBytes: maxBodyBytes}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/api/v1/providers", func(r chi.Router) {
		r.Get("/", h.listProviders)
		r.Post("/", h.createProvider)
		r.Patch("/{providerId}", h.updateProvider)
		r.Delete("/{providerId}", h.deleteProvider)
		r.Post("/{providerId}/test", h.testProvider)
		r.Post("/{providerId}/sync", h.syncProvider)
	})
	r.Route("/api/v1/models", func(r chi.Router) {
		r.Get("/", h.listModels)
		r.Post("/", h.createModel)
		r.Delete("/{modelId}", h.deleteModel)
	})
	r.Get("/api/v1/model-roles", h.listRoles)
	r.Put("/api/v1/model-roles/{role}", h.assignRole)
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, errors.FieldError(name, errors.FieldCodeInvalid)
	}
	return id, nil
}

func respond(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, status, data)
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListProviders(r.Context())
	respond(w, r, http.StatusOK, dto.FromProviders(out), err)
}

func (h *Handler) createProvider(w http.ResponseWriter, r *http.Request) {
	var in dto.ProviderRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CreateProvider(r.Context(), in.ToModel())
	respond(w, r, http.StatusCreated, dto.FromProvider(out), err)
}

func (h *Handler) updateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "providerId")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in dto.ProviderRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.UpdateProvider(r.Context(), id, in.ToModel())
	respond(w, r, http.StatusOK, dto.FromProvider(out), err)
}

func (h *Handler) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "providerId")
	if err == nil {
		err = h.svc.DeleteProvider(r.Context(), id)
	}
	respond(w, r, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}

func (h *Handler) testProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "providerId")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	n, err := h.svc.TestProvider(r.Context(), id)
	respond(w, r, http.StatusOK, dto.TestProviderResponse{OK: true, ModelCount: n}, err)
}

func (h *Handler) syncProvider(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "providerId")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.SyncModels(r.Context(), id)
	respond(w, r, http.StatusOK, dto.FromSyncResult(out), err)
}

func (h *Handler) listModels(w http.ResponseWriter, r *http.Request) {
	var providerID *uuid.UUID
	if raw := r.URL.Query().Get("providerId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, r, errors.FieldError("providerId", errors.FieldCodeInvalid))
			return
		}
		providerID = &id
	}
	var capability *model.Capability
	if raw := r.URL.Query().Get("capability"); raw != "" {
		c := model.Capability(raw)
		if !model.ValidCapability(c) {
			httpx.WriteError(w, r, errors.FieldError("capability", errors.FieldCodeInvalid))
			return
		}
		capability = &c
	}
	out, err := h.svc.ListModels(r.Context(), providerID, capability)
	respond(w, r, http.StatusOK, dto.FromModels(out), err)
}

func (h *Handler) createModel(w http.ResponseWriter, r *http.Request) {
	var in dto.ModelRequest
	if err := httpx.DecodeJSON(w, r, &in, h.maxBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CreateModel(r.Context(), in.ToModel())
	respond(w, r, http.StatusCreated, dto.FromModel(out), err)
}

func (h *Handler) deleteModel(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "modelId")
	if err == nil {
		err = h.svc.DeleteModel(r.Context(), id)
	}
	respond(w, r, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListRoles(r.Context())
	respond(w, r, http.StatusOK, dto.FromRoles(out), err)
}

func (h *Handler) assignRole(w http.ResponseWriter, r *http.Request) {
	var body dto.AssignRoleRequest
	if err := httpx.DecodeJSON(w, r, &body, h.maxBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	role := model.Role(chi.URLParam(r, "role"))
	if body.ModelID == uuid.Nil {
		httpx.WriteError(w, r, errors.FieldError("modelId", errors.FieldCodeRequired))
		return
	}
	err := h.svc.AssignRole(r.Context(), role, body.ModelID)
	respond(w, r, http.StatusOK, dto.RoleAssignmentResponse{Role: role, ModelID: body.ModelID}, err)
}
