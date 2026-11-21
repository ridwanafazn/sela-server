package prepublic

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves pre-public account endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the pre-public handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type consentRequest struct {
	AdultConfirmed *bool   `json:"adult_confirmed"`
	TermsVersion   *string `json:"terms_version"`
	PrivacyVersion *string `json:"privacy_version"`
}

// GetConsents handles GET /me/consents.
func (h *Handler) GetConsents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	summary, err := h.service.GetConsents(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, summary)
}

// UpdateConsents handles PUT /me/consents.
func (h *Handler) UpdateConsents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request consentRequest
	if !httpx.BindJSON(c, &request) {
		return
	}
	summary, err := h.service.UpdateConsents(c.Request.Context(), principal.UserID, ConsentInput{
		AdultConfirmed: request.AdultConfirmed,
		TermsVersion:   request.TermsVersion,
		PrivacyVersion: request.PrivacyVersion,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, summary)
}

// GetEntitlement handles GET /me/entitlements.
func (h *Handler) GetEntitlement(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	entitlement, err := h.service.GetEntitlement(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, entitlement)
}

// Export handles GET /me/export.
func (h *Handler) Export(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}
	data, err := h.service.Export(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("sela-export-%s.json", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.JSON(http.StatusOK, data)
}
