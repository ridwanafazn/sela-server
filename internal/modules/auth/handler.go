package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

// ConfirmEmailVerification handles POST /auth/email-verification/confirm.
func (h *Handler) ConfirmEmailVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request emailVerificationConfirmRequest
	if !httpx.BindJSON(c, &request) {
		return
	}
	if err := h.service.ConfirmEmailVerification(c.Request.Context(), request.Token); err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, gin.H{"email_verified": true})
}

// RequestEmailVerification handles POST /auth/email-verification/resend.
func (h *Handler) RequestEmailVerification(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	if err := h.service.RequestEmailVerification(c.Request.Context(), principal.UserID); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

// RequestPasswordReset handles POST /auth/password-reset/request.
func (h *Handler) RequestPasswordReset(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request passwordResetRequest
	if !httpx.BindJSON(c, &request) {
		return
	}
	if err := h.service.RequestPasswordReset(c.Request.Context(), h.clientIP(c), request.Email); err != nil {
		response.Error(c, err)
		return
	}
	// Always return the same response to avoid account enumeration.
	c.Status(http.StatusAccepted)
}

// ConfirmPasswordReset handles POST /auth/password-reset/confirm.
func (h *Handler) ConfirmPasswordReset(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	var request passwordResetConfirmRequest
	if !httpx.BindJSON(c, &request) {
		return
	}
	if err := h.service.ConfirmPasswordReset(c.Request.Context(), request.Token, request.NewPassword); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Sessions handles GET /auth/sessions.
func (h *Handler) Sessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	sessions, err := h.service.Sessions(c.Request.Context(), principal)
	if err != nil {
		response.Error(c, err)
		return
	}
	if sessions == nil {
		sessions = []SessionView{}
	}
	response.Data(c, http.StatusOK, sessions)
}

// RevokeOtherSessions handles DELETE /auth/sessions.
func (h *Handler) RevokeOtherSessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	if _, err := h.service.RevokeOtherSessions(c.Request.Context(), principal); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

// RevokeSession handles DELETE /auth/sessions/{sessionId}.
func (h *Handler) RevokeSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	sessionID := c.Param("sessionId")
	if _, err := uuid.Parse(sessionID); err != nil {
		response.Error(c, apierrors.NotFound("The session was not found."))
		return
	}
	if err := h.service.RevokeSession(c.Request.Context(), principal, sessionID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func (h *Handler) clientIP(c *gin.Context) string {
	return clientip.FromRequest(c.Request, h.trustedProxyHops)
}
