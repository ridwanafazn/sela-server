package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/requestid"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// RequestID assigns a request identifier and returns it in the response
// header. Client-supplied identifiers are ignored to prevent log injection.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := requestid.New()
		c.Writer.Header().Set(requestid.Header, id)
		c.Request = c.Request.WithContext(requestid.With(c.Request.Context(), id))
		c.Next()
	}
}

// AccessLog records safe request metadata. It never logs request bodies,
// query strings, or journal content.
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		log.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("request_id", requestid.From(c.Request.Context())),
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
	}
}

// Recovery converts panics into the canonical error envelope. The stack is
// logged for diagnosis; request data is not.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			log.Error("panic recovered",
				slog.String("request_id", requestid.From(c.Request.Context())),
				slog.String("panic_type", fmt.Sprintf("%T", recovered)),
				slog.String("stack", string(debug.Stack())),
			)

			if c.Writer.Written() {
				c.Abort()
				return
			}

			response.Error(c, apierrors.Internal())
		}()

		c.Next()
	}
}
