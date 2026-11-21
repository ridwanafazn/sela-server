package search

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Handler serves the unified search endpoint.
type Handler struct {
	service *Service
}

// NewHandler creates the search handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Search handles GET /search.
func (h *Handler) Search(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	principal, ok := authn.From(c.Request.Context())
	if !ok {
		response.Error(c, apierrors.Unauthenticated("Authentication is required."))
		return
	}

	filter := Filter{
		Query: strings.TrimSpace(c.Query("q")),
		Types: splitTypes(c.Query("types")),
		Limit: defaultLimit,
	}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxLimit {
			response.Error(c, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "limit",
				Code:    "INVALID_VALUE",
				Message: "Limit must be an integer between 1 and 20.",
			}))
			return
		}
		filter.Limit = limit
	}

	result, err := h.service.Search(c.Request.Context(), principal.UserID, filter)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Data(c, http.StatusOK, result)
}

func splitTypes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
