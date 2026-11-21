package journal

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the journal endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the journal handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Create handles POST /journals.
func (h *Handler) Create(c *gin.Context) {
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

	var request createRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	view, status, err := h.service.Create(c.Request.Context(), principal.UserID, idempotencyKey, CreateInput{
		Title:     request.Title,
		Content:   request.Content,
		EntryDate: request.EntryDate,
		Status:    request.Status,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, status, view)
}

// List handles GET /journals.
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

// Changes handles GET /journal-changes.
func (h *Handler) Changes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	filter, apiErr := parseChangeQuery(c)
	if apiErr != nil {
		response.Error(c, apiErr)
		return
	}

	page, err := h.service.Changes(c.Request.Context(), principal.UserID, filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, changeListResponse{
		Data: page.Items,
		Pagination: response.Pagination{
			NextCursor: page.NextCursor,
			HasMore:    page.HasMore,
		},
		Sync: changeSync{ServerTime: page.ServerTime},
	})
}

type changeSync struct {
	ServerTime time.Time `json:"server_time"`
}

type changeListResponse struct {
	Data       []JournalChange     `json:"data"`
	Pagination response.Pagination `json:"pagination"`
	Sync       changeSync          `json:"sync"`
}

// Get handles GET /journals/{journalId}.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID := c.Param("journalId")
	if _, err := uuid.Parse(journalID); err != nil {
		response.Error(c, apierrors.NotFound("The journal was not found."))
		return
	}

	view, err := h.service.Get(c.Request.Context(), principal.UserID, journalID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, view)
}

// Update handles PATCH /journals/{journalId}.
func (h *Handler) Update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID := c.Param("journalId")
	if _, err := uuid.Parse(journalID); err != nil {
		response.Error(c, apierrors.NotFound("The journal was not found."))
		return
	}

	ifMatch, ok := requireIfMatch(c)
	if !ok {
		return
	}

	var request updateRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	view, err := h.service.Update(c.Request.Context(), principal.UserID, journalID, ifMatch, UpdateInput{
		Title:     request.Title,
		Content:   request.Content,
		EntryDate: request.EntryDate,
		Status:    request.Status,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, view)
}

// Delete handles DELETE /journals/{journalId}.
func (h *Handler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID := c.Param("journalId")
	if _, err := uuid.Parse(journalID); err != nil {
		response.Error(c, apierrors.NotFound("The journal was not found."))
		return
	}

	ifMatch, ok := requireIfMatch(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), principal.UserID, journalID, ifMatch); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
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

func requireIfMatch(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if raw == "" {
		response.Error(c, apierrors.PreconditionRequired("The If-Match header is required."))
		return 0, false
	}

	version, err := strconv.Atoi(raw)
	if err != nil || version < 1 {
		response.Error(c, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "If-Match",
			Code:    "INVALID_FORMAT",
			Message: "If-Match must be a positive integer.",
		}))
		return 0, false
	}

	return version, true
}

func parseChangeQuery(c *gin.Context) (ChangeFilter, *apierrors.Error) {
	filter := ChangeFilter{Limit: 20}
	var details []apierrors.Detail

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			details = append(details, apierrors.Detail{
				Field:   "limit",
				Code:    "INVALID_VALUE",
				Message: "Limit must be an integer between 1 and 100.",
			})
		} else {
			filter.Limit = limit
		}
	}

	filter.Cursor = strings.TrimSpace(c.Query("cursor"))

	if len(details) > 0 {
		return filter, apierrors.Validation("The request is invalid.", details...)
	}
	return filter, nil
}

func parseListQuery(c *gin.Context) (ListFilter, *apierrors.Error) {
	filter := ListFilter{Limit: 20}
	var details []apierrors.Detail

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
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
	case StatusActive, StatusDraft:
		filter.Status = status
	default:
		details = append(details, apierrors.Detail{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "Status must be all, active, or draft.",
		})
	}

	query := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(query) > 200 {
		details = append(details, apierrors.Detail{
			Field:   "q",
			Code:    "TOO_LONG",
			Message: "Search query must be at most 200 characters.",
		})
	} else {
		filter.Query = query
	}

	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			details = append(details, apierrors.Detail{
				Field:   "from",
				Code:    "INVALID_FORMAT",
				Message: "From date must use YYYY-MM-DD.",
			})
		} else {
			filter.From = &value
		}
	}

	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			details = append(details, apierrors.Detail{
				Field:   "to",
				Code:    "INVALID_FORMAT",
				Message: "To date must use YYYY-MM-DD.",
			})
		} else {
			filter.To = &value
		}
	}

	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		details = append(details, apierrors.Detail{
			Field:   "from",
			Code:    "INVALID_RANGE",
			Message: "From date must not be after the to date.",
		})
	}

	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		filter.Cursor = cursor
	}

	if len(details) > 0 {
		return filter, apierrors.Validation("The request is invalid.", details...)
	}

	return filter, nil
}
