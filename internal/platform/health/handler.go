// Package health exposes liveness and readiness endpoints.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/requestid"
	"github.com/ridwanafazn/sela-server/internal/shared/response"
)

// Readiness reports dependency health. A non-nil error means the service is
// not ready to receive traffic.
type Readiness interface {
	Ready(ctx context.Context) (map[string]string, error)
}

// Handler serves the health endpoints.
type Handler struct {
	readiness Readiness
	timeout   time.Duration
	log       *slog.Logger
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// NewHandler creates a health handler with a bounded readiness budget.
func NewHandler(readiness Readiness, timeout time.Duration, log *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}

	return &Handler{readiness: readiness, timeout: timeout, log: log}
}

// Live confirms that the process is running. It never checks dependencies.
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}

// Ready confirms that the service can serve traffic. Dependency details are
// logged but never returned to the client.
func (h *Handler) Ready(c *gin.Context) {
	if h.readiness == nil {
		h.log.Error("readiness checker is not configured",
			slog.String("request_id", requestid.From(c.Request.Context())),
		)
		response.Error(c, apierrors.Internal())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	checks, err := h.readiness.Ready(ctx)
	if err != nil {
		h.log.Warn("readiness check failed",
			slog.String("request_id", requestid.From(c.Request.Context())),
			slog.Any("checks", checks),
			slog.String("error", err.Error()),
		)
		response.Error(c, apierrors.UpstreamUnavailable("Service is not ready."))
		return
	}

	c.JSON(http.StatusOK, healthResponse{Status: "ok", Checks: checks})
}
