package llm

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// ResolvedModelResponse includes the decrypted API key; only the service-token route returns it.
type ResolvedModelResponse struct {
	Role          Role         `json:"role"`
	ProviderKind  ProviderKind `json:"providerKind"`
	BaseURL       string       `json:"baseUrl"`
	APIKey        string       `json:"apiKey"`
	ModelRef      string       `json:"modelRef"`
	Capabilities  []Capability `json:"capabilities"`
	EmbeddingDims *int         `json:"embeddingDims,omitempty"`
	FallbackFrom  Role         `json:"fallbackFrom,omitempty"`
}

func newResolvedModelResponse(resolved ResolvedModel) ResolvedModelResponse {
	return ResolvedModelResponse(resolved)
}

type roleResolver interface {
	Resolve(ctx context.Context, role Role) (ResolvedModel, error)
}

// ResolveController serves the internal GET /internal/models/resolve?role=... route.
type ResolveController struct{ resolver roleResolver }

func NewResolveController(resolver roleResolver) *ResolveController {
	return &ResolveController{resolver: resolver}
}

func (h *ResolveController) HandleResolve(c *gin.Context) {
	c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	resolved, err := h.resolver.Resolve(c.Request.Context(), Role(c.Query("role")))
	httpx.Respond(c, http.StatusOK, newResolvedModelResponse(resolved), err)
}
