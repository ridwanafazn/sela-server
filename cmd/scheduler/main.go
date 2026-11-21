// Command scheduler enqueues due weekly and monthly insights on an interval.
// It is the non-GCP alternative to Cloud Scheduler for a long-running host.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "scheduler: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	interval := flag.Duration("interval", 5*time.Minute, "fan-out interval")
	once := flag.Bool("once", false, "run one fan-out and exit")
	flag.Parse()

	if *interval < 30*time.Second {
		return fmt.Errorf("interval must be at least 30s")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log, cfg.App.Name, "scheduler")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	usageService := aiusage.NewService(
		db.Gorm,
		aiusage.Prices{
			InputPerMillion:     cfg.AI.InputCostPerMillion,
			OutputPerMillion:    cfg.AI.OutputCostPerMillion,
			EmbeddingPerMillion: cfg.AI.EmbeddingCostPerMillion,
		},
		aiusage.Budgets{
			UserMonthlyUSD:   cfg.AI.UserMonthlyBudgetUSD,
			GlobalMonthlyUSD: cfg.AI.GlobalMonthlyBudgetUSD,
		},
		log,
	)

	service := insight.NewScheduleService(
		insight.NewRepository(db.Gorm),
		job.NewQueue(db.Gorm),
		usageService,
		log,
	)

	runOnce := func() {
		if _, err := service.FanOutDue(ctx); err != nil {
			log.ErrorContext(ctx, "scheduled insight fan-out failed",
				slog.String("error", err.Error()),
			)
		}
	}

	if *once {
		runOnce()
		return nil
	}

	log.Info("scheduler started", slog.Duration("interval", *interval))
	runOnce()

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler stopped")
			return nil
		case <-ticker.C:
			runOnce()
		}
	}
}
