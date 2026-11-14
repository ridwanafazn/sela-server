package memory

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

// Handler serves the memory endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the memory handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List handles GET /memories.
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

// Get handles GET /memories/{memoryId}.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	memoryID, ok := requireMemoryID(c)
	if !ok {
		return
	}

	detail, err := h.service.Get(c.Request.Context(), principal.UserID, memoryID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, detail)
}

// Update handles PATCH /memories/{memoryId}.
func (h *Handler) Update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	memoryID, ok := requireMemoryID(c)
	if !ok {
		return
	}

	var request updateMemoryRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	detail, err := h.service.Update(c.Request.Context(), principal.UserID, memoryID, UpdateInput{
		Content:     request.Content,
		Category:    request.Category.Value,
		CategorySet: request.Category.Set,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, detail)
}

// Delete handles DELETE /memories/{memoryId}.
func (h *Handler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	memoryID, ok := requireMemoryID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), principal.UserID, memoryID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// Feedback handles POST /memories/{memoryId}/feedback.
func (h *Handler) Feedback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	memoryID, ok := requireMemoryID(c)
	if !ok {
		return
	}

	var request feedbackRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	detail, err := h.service.Feedback(c.Request.Context(), principal.UserID, memoryID, FeedbackInput{
		Action:           request.Action,
		CorrectedContent: request.CorrectedContent,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, detail)
}

func requireMemoryID(c *gin.Context) (string, bool) {
	value := c.Param("memoryId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, memoryNotFound())
		return "", false
	}
	return value, true
}

func parseListQuery(c *gin.Context) (ListFilter, *apierrors.Error) {
	filter := ListFilter{Limit: defaultPageLimit}
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

	switch status := strings.TrimSpace(c.Query("status")); status {
	case "", "all":
		filter.Status = "all"
	case StatusActive, StatusCandidate, StatusRejected, StatusSuperseded:
		filter.Status = status
	default:
		details = append(details, apierrors.Detail{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "Status must be all, active, candidate, rejected, or superseded.",
		})
	}

	category := strings.TrimSpace(c.Query("category"))
	if len([]rune(category)) > maxCategoryLength {
		details = append(details, apierrors.Detail{
			Field:   "category",
			Code:    "TOO_LONG",
			Message: "Category must be at most 100 characters.",
		})
	} else {
		filter.Category = category
	}

	filter.Cursor = strings.TrimSpace(c.Query("cursor"))

	if len(details) > 0 {
		return filter, apierrors.Validation("The request is invalid.", details...)
	}
	return filter, nil
}
