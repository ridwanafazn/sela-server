package httpx

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// RequireIdempotencyKey parses the UUID Idempotency-Key header and writes a
// validation error when it is missing or malformed.
func RequireIdempotencyKey(c *gin.Context) (string, bool) {
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
