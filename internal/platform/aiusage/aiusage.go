// Package aiusage records AI token usage, latency, and estimated cost and
// enforces monthly budgets.
package aiusage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

// Operation names.
const (
	OperationCompanion       = "companion"
	OperationMemoryJournal   = "memory.extraction.journal"
	OperationMemoryMessage   = "memory.extraction.message"
	OperationInsight         = "insight.generate"
	OperationJournalAnalysis = "journal.analysis"
	OperationEmbedding       = "embedding.journal"
)

// Event is one AI operation's operational metadata. It never contains prompt
// or source content.
type Event struct {
	UserID        string
	Operation     string
	Provider      string
	Model         string
	InputTokens   int
	OutputTokens  int
	Latency       time.Duration
	Success       bool
	ErrorCategory string
	ReferenceID   *string
}

// Prices holds per-million-token rates in USD.
type Prices struct {
	InputPerMillion     float64
	OutputPerMillion    float64
	EmbeddingPerMillion float64
}

// Budgets holds monthly spending limits. Zero disables a limit.
type Budgets struct {
	UserMonthlyUSD   float64
	GlobalMonthlyUSD float64
}

// Service records usage and checks budgets.
type Service struct {
	db      *gorm.DB
	prices  Prices
	budgets Budgets
	log     *slog.Logger
}

// NewService creates the usage service.
func NewService(db *gorm.DB, prices Prices, budgets Budgets, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, prices: prices, budgets: budgets, log: log}
}

// Record persists one usage event. Failures are logged but never break the AI
// operation.
func (s *Service) Record(ctx context.Context, event Event) {
	if s == nil || s.db == nil {
		return
	}
	if event.InputTokens < 0 {
		event.InputTokens = 0
	}
	if event.OutputTokens < 0 {
		event.OutputTokens = 0
	}
	if event.Latency < 0 {
		event.Latency = 0
	}

	id, err := ids.New()
	if err != nil {
		s.log.Warn("ai usage id generation failed", slog.String("error", err.Error()))
		return
	}

	cost := s.EstimateCost(event.Operation, event.InputTokens, event.OutputTokens)
	recordCtx := context.WithoutCancel(ctx)
	if err := s.db.WithContext(recordCtx).Exec(
		`INSERT INTO ai_usage_events
		 (id, user_id, operation, provider, model, input_tokens, output_tokens,
		  estimated_cost_usd, latency_ms, success, error_category, reference_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		event.UserID,
		event.Operation,
		event.Provider,
		event.Model,
		event.InputTokens,
		event.OutputTokens,
		cost,
		event.Latency.Milliseconds(),
		event.Success,
		nullString(event.ErrorCategory),
		event.ReferenceID,
		time.Now().UTC(),
	).Error; err != nil {
		s.log.Warn("ai usage recording failed",
			slog.String("operation", event.Operation),
			slog.String("error", err.Error()),
		)
	}
}

// Exhausted reports whether the user or global monthly budget is exhausted.
func (s *Service) Exhausted(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	if s.budgets.UserMonthlyUSD <= 0 && s.budgets.GlobalMonthlyUSD <= 0 {
		return false, nil
	}

	if s.budgets.UserMonthlyUSD > 0 {
		var userCost float64
		if err := s.db.WithContext(ctx).Raw(
			`SELECT COALESCE(SUM(estimated_cost_usd), 0)
			 FROM ai_usage_events
			 WHERE user_id = ? AND created_at >= date_trunc('month', now())`,
			userID,
		).Scan(&userCost).Error; err != nil {
			return false, fmt.Errorf("read user ai budget: %w", err)
		}
		if userCost >= s.budgets.UserMonthlyUSD {
			return true, nil
		}
	}

	if s.budgets.GlobalMonthlyUSD > 0 {
		var globalCost float64
		if err := s.db.WithContext(ctx).Raw(
			`SELECT COALESCE(SUM(estimated_cost_usd), 0)
			 FROM ai_usage_events
			 WHERE created_at >= date_trunc('month', now())`,
		).Scan(&globalCost).Error; err != nil {
			return false, fmt.Errorf("read global ai budget: %w", err)
		}
		if globalCost >= s.budgets.GlobalMonthlyUSD {
			return true, nil
		}
	}

	return false, nil
}

// EstimateCost returns the configured estimated cost for one operation.
func (s *Service) EstimateCost(operation string, inputTokens, outputTokens int) float64 {
	if s == nil {
		return 0
	}
	if operation == OperationEmbedding {
		return float64(inputTokens) / 1_000_000 * s.prices.EmbeddingPerMillion
	}
	return float64(inputTokens)/1_000_000*s.prices.InputPerMillion +
		float64(outputTokens)/1_000_000*s.prices.OutputPerMillion
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
