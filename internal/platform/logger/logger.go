// Package logger builds the structured logger used by the API.
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/ridwanafazn/sela-server/internal/config"
)

// New returns a structured logger. Logs never include request or journal
// content by construction; callers are responsible for choosing safe fields.
func New(cfg config.Log, service, version string) *slog.Logger {
	options := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}

	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("version", version),
	)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
