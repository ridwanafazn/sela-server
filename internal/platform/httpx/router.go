// Package httpx wires the HTTP router, middleware, and operational routes.
package httpx

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/platform/health"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// RouterOptions configures the API router.
type RouterOptions struct {
	Env              string
	Log              *slog.Logger
	Readiness        health.Readiness
	ReadinessTimeout time.Duration
}

// NewRouter builds the Gin engine with operational middleware and routes.
func NewRouter(opts RouterOptions) *gin.Engine {
	switch opts.Env {
	case config.EnvProduction:
		gin.SetMode(gin.ReleaseMode)
	case config.EnvTest:
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	router := gin.New()
	router.Use(RequestID(), AccessLog(log), Recovery(log))

	router.NoRoute(func(c *gin.Context) {
		response.Error(c, apierrors.NotFound("The requested resource was not found."))
	})

	healthHandler := health.NewHandler(opts.Readiness, opts.ReadinessTimeout, log)

	api := router.Group("/api/v1")
	api.GET("/health/live", healthHandler.Live)
	api.GET("/health/ready", healthHandler.Ready)

	return router
}
