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

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/internal/platform/logger"
	"github.com/ridwanafazn/sela-server/prompts"
)

const backfillPageSize = 200

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	once := flag.Bool("once", false, "process the currently available jobs and exit")
	backfill := flag.Bool("backfill", false, "enqueue embedding and memory extraction jobs for all active journals, then exit")
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

	if *backfill {
		return runBackfill(ctx, cfg, db, log)
	}

	embeddings, err := ai.NewEmbedding(ctx, cfg.AI)
	if err != nil {
		return err
	}
	llm, err := ai.NewLLM(cfg.AI)
	if err != nil {
		return err
	}

	embeddingModel := cfg.AI.EmbeddingModelName()

	embeddingService := journal.NewEmbeddingService(
		journal.NewRepository(db.Gorm),
		embeddings,
		embeddingModel,
		cfg.AI.EmbeddingDimension,
		log,
	)
	extractionService := memory.NewExtractionService(
		memory.NewRepository(db.Gorm),
		llm,
		cfg.AI.Model,
		log,
	)
	insightGeneration := insight.NewGenerationService(
		insight.NewRepository(db.Gorm),
		llm,
		cfg.AI.Model,
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
		job.TypeJournalMemoryExtraction: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				JournalID string `json:"journal_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return fmt.Errorf("%w: decode memory extraction payload: %v", ai.ErrInvalidRequest, err)
			}
			if payload.JournalID == "" {
				return fmt.Errorf("%w: memory extraction payload is missing journal_id", ai.ErrInvalidRequest)
			}
			return extractionService.ProcessJournal(ctx, payload.JournalID)
		}),
		job.TypeMessageMemoryExtraction: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return fmt.Errorf("%w: decode message extraction payload: %v", ai.ErrInvalidRequest, err)
			}
			if payload.MessageID == "" {
				return fmt.Errorf("%w: message extraction payload is missing message_id", ai.ErrInvalidRequest)
			}
			return extractionService.ProcessMessage(ctx, payload.MessageID)
		}),
		job.TypeInsightGenerate: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			if claimed.UserID == nil || *claimed.UserID == "" {
				return fmt.Errorf("%w: insight job is missing user_id", ai.ErrInvalidRequest)
			}
			var payload insight.JobPayload
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return fmt.Errorf("%w: decode insight payload: %v", ai.ErrInvalidRequest, err)
			}
			result, err := insightGeneration.Process(ctx, claimed.ID, *claimed.UserID, payload)
			if err != nil {
				return err
			}
			return queue.SetResult(ctx, claimed.ID, result)
		}),
	}

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	worker := job.NewWorker(queue, handlers, workerID, 2*time.Second, log)

	log.Info("worker started",
		slog.String("worker_id", workerID),
		slog.String("embedding_provider", cfg.AI.EmbeddingProvider),
		slog.String("embedding_model", embeddingModel),
		slog.String("memory_prompt_version", prompts.MemoryExtractionVersion),
		slog.String("insight_prompt_version", prompts.InsightVersion),
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

func runBackfill(ctx context.Context, cfg *config.Config, db *database.DB, log *slog.Logger) error {
	repository := journal.NewRepository(db.Gorm)
	queue := job.NewQueue(db.Gorm)
	embeddingModel := cfg.AI.EmbeddingModelName()
	now := time.Now().UTC()

	var afterID string
	var journals int
	for {
		targets, err := repository.ActiveForBackfill(ctx, afterID, backfillPageSize)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			break
		}

		for _, target := range targets {
			hash := journal.ContentHash(target.Content)
			err := db.Gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := queue.EnqueueJournalEmbeddingForModel(
					ctx, tx, target.UserID, target.ID, hash, embeddingModel, now,
				); err != nil {
					return err
				}
				return queue.EnqueueJournalMemoryExtractionForVersion(
					ctx, tx, target.UserID, target.ID, hash, prompts.MemoryExtractionVersion, now,
				)
			})
			if err != nil {
				return err
			}
			journals++
		}

		afterID = targets[len(targets)-1].ID
	}

	log.Info("backfill jobs enqueued",
		slog.Int("journals", journals),
		slog.String("embedding_model", embeddingModel),
		slog.String("memory_prompt_version", prompts.MemoryExtractionVersion),
	)
	return nil
}
