package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/shared/authn"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Authenticate validates the bearer token and the session's revocation state
// in the database before allowing a protected request.
func Authenticate(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")

		header := c.GetHeader("Authorization")
		scheme, raw, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(raw) == "" {
			response.Error(c, errUnauthenticated)
			c.Abort()
			return
		}

		claims, err := service.ParseAccessToken(strings.TrimSpace(raw))
		if err != nil {
			response.Error(c, errUnauthenticated)
			c.Abort()
			return
		}

		principal, err := service.Authenticate(c.Request.Context(), claims)
		if err != nil {
			response.Error(c, err)
			c.Abort()
			return
		}

		c.Request = c.Request.WithContext(authn.With(c.Request.Context(), principal))
		c.Next()
	}
}
