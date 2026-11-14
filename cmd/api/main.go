package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ridwanafazn/sela-server/internal/app"
	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/platform/logger"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log, cfg.App.Name, version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, log)
	if err != nil {
		log.Error("startup failed", slog.String("error", err.Error()))
		return err
	}

	log.Info("starting sela-server", slog.Int("port", cfg.App.Port))
	return application.Run(ctx)
}
