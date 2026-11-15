package insight

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the insight endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the insight handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Request handles POST /insights.
func (h *Handler) Request(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	idempotencyKey, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}

	var request generateRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	focus := ""
	if request.Focus != nil {
		focus = *request.Focus
	}

	accepted, err := h.service.Request(c.Request.Context(), principal.UserID, idempotencyKey, GenerateInput{
		Period:    request.Period,
		StartDate: request.StartDate,
		EndDate:   request.EndDate,
		Focus:     focus,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Header("Retry-After", strconv.Itoa(accepted.RetryAfterSeconds))
	response.Data(c, http.StatusAccepted, accepted)
}

// List handles GET /insights.
func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	filter, apiErr := parseListQuery(c)
	if apiErr != nil {
		response.Error(c, apiErr)
		return
	}

	page, err := h.service.List(c.Request.Context(), principal.UserID, filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Collection(c, page.Items, response.Pagination{
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	})
}

// Get handles GET /insights/{insightId}.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	insightID, ok := requireInsightID(c)
	if !ok {
		return
	}

	detail, err := h.service.Get(c.Request.Context(), principal.UserID, insightID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, detail.View)
}

// Delete handles DELETE /insights/{insightId}.
func (h *Handler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	insightID, ok := requireInsightID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), principal.UserID, insightID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// Feedback handles POST /insights/{insightId}/feedback.
func (h *Handler) Feedback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	insightID, ok := requireInsightID(c)
	if !ok {
		return
	}

	var request feedbackRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	detail, err := h.service.Feedback(c.Request.Context(), principal.UserID, insightID, FeedbackInput{
		Action: request.Action,
		Note:   request.Note,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, detail.View)
}

// JobStatus handles GET /insight-jobs/{jobId}.
func (h *Handler) JobStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	jobID, ok := requireJobID(c)
	if !ok {
		return
	}

	view, err := h.service.JobStatus(c.Request.Context(), principal.UserID, jobID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Header("Retry-After", strconv.Itoa(view.RetryAfterSeconds))
	response.Data(c, http.StatusOK, view)
}

func requireIdempotencyKey(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if raw == "" {
		response.Error(c, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "Idempotency-Key",
			Code:    "REQUIRED",
			Message: "The Idempotency-Key header is required.",
		}))
		return "", false
	}

	parsed, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "Idempotency-Key",
			Code:    "INVALID_FORMAT",
			Message: "The Idempotency-Key header must be a UUID.",
		}))
		return "", false
	}
	return parsed.String(), true
}

func requireInsightID(c *gin.Context) (string, bool) {
	value := c.Param("insightId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, insightNotFound())
		return "", false
	}
	return value, true
}

func requireJobID(c *gin.Context) (string, bool) {
	value := c.Param("jobId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, insightNotFound())
		return "", false
	}
	return value, true
}

func parseListQuery(c *gin.Context) (ListFilter, *apierrors.Error) {
	filter := ListFilter{Period: "all", Limit: defaultPageLimit}
	var details []apierrors.Detail

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxPageLimit {
			details = append(details, apierrors.Detail{
				Field:   "limit",
				Code:    "INVALID_VALUE",
				Message: "Limit must be an integer between 1 and 100.",
			})
		} else {
			filter.Limit = limit
		}
	}

	switch period := strings.TrimSpace(c.Query("period")); period {
	case "", "all":
		filter.Period = "all"
	case GenerationWeekly, GenerationMonthly:
		filter.Period = period
	default:
		details = append(details, apierrors.Detail{
			Field:   "period",
			Code:    "INVALID_VALUE",
			Message: "Period must be all, weekly, or monthly.",
		})
	}

	filter.Cursor = strings.TrimSpace(c.Query("cursor"))

	if len(details) > 0 {
		return filter, apierrors.Validation("The request is invalid.", details...)
	}
	return filter, nil
}
