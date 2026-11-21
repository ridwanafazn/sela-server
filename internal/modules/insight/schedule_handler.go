package insight

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// ScheduleHandler serves the insight schedule endpoints.
type ScheduleHandler struct {
	service *ScheduleService
}

// NewScheduleHandler creates the schedule handler.
func NewScheduleHandler(service *ScheduleService) *ScheduleHandler {
	return &ScheduleHandler{service: service}
}

type scheduleRequest struct {
	Enabled *bool   `json:"enabled" binding:"required"`
	Cadence *string `json:"cadence"`
}

// Get handles GET /insight-schedule.
func (h *ScheduleHandler) Get(c *gin.Context) {
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

// Put handles PUT /insight-schedule.
func (h *ScheduleHandler) Put(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request scheduleRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	cadence := ""
	if request.Cadence != nil {
		cadence = *request.Cadence
	}
	view, err := h.service.Set(c.Request.Context(), principal.UserID, ScheduleInput{
		Enabled: *request.Enabled,
		Cadence: cadence,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, view)
}

// Delete handles DELETE /insight-schedule.
func (h *ScheduleHandler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	view, err := h.service.Disable(c.Request.Context(), principal.UserID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, view)
}
