package journalanalysis

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the per-entry journal analysis endpoints.
type Handler struct {
	service *Service
}

// NewHandler creates the journal analysis handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Request handles POST /journals/{journalId}/analysis.
func (h *Handler) Request(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID, ok := requireJournalID(c)
	if !ok {
		return
	}
	idempotencyKey, ok := httpx.RequireIdempotencyKey(c)
	if !ok {
		return
	}

	accepted, err := h.service.Request(c.Request.Context(), principal.UserID, journalID, idempotencyKey)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Header("Retry-After", strconv.Itoa(accepted.RetryAfterSeconds))
	response.Data(c, http.StatusAccepted, accepted)
}

// Get handles GET /journals/{journalId}/analysis.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID, ok := requireJournalID(c)
	if !ok {
		return
	}

	view, err := h.service.Get(c.Request.Context(), principal.UserID, journalID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, view)
}

// Delete handles DELETE /journals/{journalId}/analysis.
func (h *Handler) Delete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	journalID, ok := requireJournalID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), principal.UserID, journalID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

// JobStatus handles GET /journal-analysis-jobs/{jobId}.
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

func requireJournalID(c *gin.Context) (string, bool) {
	value := c.Param("journalId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, journalNotFound())
		return "", false
	}
	return value, true
}

func requireJobID(c *gin.Context) (string, bool) {
	value := c.Param("jobId")
	if _, err := uuid.Parse(value); err != nil {
		response.Error(c, analysisNotFound())
		return "", false
	}
	return value, true
}
