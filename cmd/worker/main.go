package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	once := flag.Bool("once", false, "process the currently available jobs and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log, cfg.App.Name, "worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	embeddings, err := ai.NewEmbedding(ctx, cfg.AI)
	if err != nil {
		return err
	}

	embeddingModel := cfg.AI.EmbeddingModel
	switch cfg.AI.EmbeddingProvider {
	case "vertex":
		embeddingModel = cfg.AI.VertexEmbeddingModel
	case "fake":
		if embeddingModel == "" {
			embeddingModel = "fake-embedding-768"
		}
	}

	embeddingService := journal.NewEmbeddingService(
		journal.NewRepository(db.Gorm),
		embeddings,
		embeddingModel,
		cfg.AI.EmbeddingDimension,
		log,
	)

	queue := job.NewQueue(db.Gorm)
	handlers := map[string]job.Handler{
		job.TypeJournalEmbedding: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				JournalID string `json:"journal_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return fmt.Errorf("%w: decode embedding payload: %v", ai.ErrInvalidRequest, err)
			}
			if payload.JournalID == "" {
				return fmt.Errorf("%w: embedding payload is missing journal_id", ai.ErrInvalidRequest)
			}
			return embeddingService.ProcessJournal(ctx, payload.JournalID)
		}),
	}

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	worker := job.NewWorker(queue, handlers, workerID, 2*time.Second, log)

	log.Info("worker started",
		slog.String("worker_id", workerID),
		slog.String("embedding_provider", cfg.AI.EmbeddingProvider),
		slog.String("embedding_model", embeddingModel),
	)

	if *once {
		for {
			processed, err := worker.ProcessOnce(ctx)
			if err != nil {
				return err
			}
			if !processed {
				return nil
			}
		}
	}

	return worker.Run(ctx)
}
