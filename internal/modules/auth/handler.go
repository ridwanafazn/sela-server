package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/clientip"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the public authentication endpoints.
type Handler struct {
	service          *Service
	trustedProxyHops int
}

// NewHandler creates the authentication handler.
func NewHandler(service *Service, trustedProxyHops int) *Handler {
	return &Handler{service: service, trustedProxyHops: trustedProxyHops}
}

// Register handles POST /auth/register.
func (h *Handler) Register(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request registerRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	result, err := h.service.Register(c.Request.Context(), h.clientIP(c), RegisterInput{
		Email:       request.Email,
		Password:    request.Password,
		DisplayName: request.DisplayName,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusCreated, newAuthData(result))
}

// Login handles POST /auth/login.
func (h *Handler) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request loginRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	result, err := h.service.Login(c.Request.Context(), h.clientIP(c), LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, newAuthData(result))
}

// Refresh handles POST /auth/refresh.
func (h *Handler) Refresh(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request refreshRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), h.clientIP(c), request.RefreshToken)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, newAuthData(result))
}

// Logout handles POST /auth/logout. It requires authentication.
func (h *Handler) Logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	if err := h.service.Logout(c.Request.Context(), principal); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

func (h *Handler) clientIP(c *gin.Context) string {
	return clientip.FromRequest(c.Request, h.trustedProxyHops)
}
