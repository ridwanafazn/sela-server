package privacy

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the privacy preferences endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the privacy handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type updateRequest struct {
	AIContextMode           *string `json:"ai_context_mode"`
	MemoryExtractionEnabled *bool   `json:"memory_extraction_enabled"`
}

// Get handles GET /me/privacy.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	view, err := h.service.Get(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, view)
}

// Update handles PUT /me/privacy.
func (h *Handler) Update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request updateRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	view, err := h.service.Update(c.Request.Context(), principal.UserID, UpdateInput{
		AIContextMode:           request.AIContextMode,
		MemoryExtractionEnabled: request.MemoryExtractionEnabled,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, view)
}
