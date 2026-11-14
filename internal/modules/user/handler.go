package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the current-user endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the user handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetMe handles GET /me.
func (h *Handler) GetMe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	me, err := h.service.GetMe(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, newMeDTO(me))
}

// UpdateMe handles PATCH /me.
func (h *Handler) UpdateMe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request updateMeRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	me, err := h.service.UpdateMe(c.Request.Context(), principal.UserID, ProfilePatch{
		DisplayName:        request.DisplayName,
		Language:           request.Language,
		Timezone:           request.Timezone,
		CommunicationStyle: request.CommunicationStyle,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, newMeDTO(me))
}
