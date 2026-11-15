package account

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves account lifecycle endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the account handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// DeleteMe handles DELETE /me.
func (h *Handler) DeleteMe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request deleteAccountRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	view, err := h.service.DeleteAccount(
		c.Request.Context(),
		principal.UserID,
		request.Password,
		request.Confirmation,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusAccepted, view)
}
