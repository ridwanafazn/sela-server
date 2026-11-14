package conversation

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

// Handler serves the Companion endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the conversation handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Create handles POST /conversations.
func (h *Handler) Create(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	var request createConversationRequest
	if c.Request.ContentLength != 0 {
		if !httpx.BindJSON(c, &request) {
			return
		}
	}

	view, err := h.service.CreateConversation(c.Request.Context(), principal.UserID, request.Title)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusCreated, view)
}

// List handles GET /conversations.
func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	limit, apiErr := parseLimit(c)
	if apiErr != nil {
		response.Error(c, apiErr)
		return
	}

	page, err := h.service.ListConversations(c.Request.Context(), principal.UserID, c.Query("cursor"), limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Collection(c, page.Items, response.Pagination{
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	})
}

// Get handles GET /conversations/{conversationId}.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	conversationID, ok := conversationID(c)
	if !ok {
		return
	}

	view, err := h.service.GetConversation(c.Request.Context(), principal.UserID, conversationID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, view)
}

// Delete handles DELETE /conversations/{conversationId}.
func (h *Handler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	conversationID, ok := conversationID(c)
	if !ok {
		return
	}

	if err := h.service.DeleteConversation(c.Request.Context(), principal.UserID, conversationID); err != nil {
		response.Error(c, err)
		return
	}

	response.NoContent(c)
}

// ListMessages handles GET /conversations/{conversationId}/messages.
func (h *Handler) ListMessages(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	conversationID, ok := conversationID(c)
	if !ok {
		return
	}

	limit, apiErr := parseLimit(c)
	if apiErr != nil {
		response.Error(c, apiErr)
		return
	}

	page, err := h.service.ListMessages(
		c.Request.Context(),
		principal.UserID,
		conversationID,
		c.Query("before"),
		limit,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Collection(c, page.Items, response.Pagination{
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	})
}

// SendMessage handles POST /conversations/{conversationId}/messages.
func (h *Handler) SendMessage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	conversationID, ok := conversationID(c)
	if !ok {
		return
	}

	idempotencyKey, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}

	var request sendMessageRequest
	if !httpx.BindJSON(c, &request) {
		return
	}

	payload, err := h.service.SendMessage(
		c.Request.Context(),
		principal.UserID,
		conversationID,
		idempotencyKey,
		request.Content,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Data(c, http.StatusOK, payload)
}

func conversationID(c *gin.Context) (string, bool) {
	value := c.Param("conversationId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, conversationNotFound())
		return "", false
	}
	return value, true
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

func parseLimit(c *gin.Context) (int, *apierrors.Error) {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return defaultPageLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > conversationPageLimit {
		return 0, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "Limit must be an integer between 1 and 100.",
		})
	}
	return limit, nil
}
