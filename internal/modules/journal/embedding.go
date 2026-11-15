package journal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
)

// EmbeddingService generates and replaces journal vectors.
type EmbeddingService struct {
	repo       *Repository
	embeddings ai.EmbeddingProvider
	model      string
	provider   string
	dimension  int
	options    ChunkOptions
	usage      *aiusage.Service
	log        *slog.Logger
}

// NewEmbeddingService creates the embedding pipeline.
func NewEmbeddingService(
	repo *Repository,
	embeddings ai.EmbeddingProvider,
	model, provider string,
	dimension int,
	usage *aiusage.Service,
	log *slog.Logger,
) *EmbeddingService {
	if dimension <= 0 {
		dimension = 768
	}
	if log == nil {
		log = slog.Default()
	}
	return &EmbeddingService{
		repo:       repo,
		embeddings: embeddings,
		model:      model,
		provider:   provider,
		dimension:  dimension,
		options:    DefaultChunkOptions(),
		usage:      usage,
		log:        log,
	}
}

// ProcessJournal chunks, embeds, and atomically replaces a journal's vectors.
func (s *EmbeddingService) ProcessJournal(ctx context.Context, journalID string) error {
	entry, err := s.repo.GetByIDForProcessing(ctx, journalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if entry.Status != StatusActive || entry.DeletedAt != nil {
		if err := s.repo.DeleteEmbeddings(ctx, journalID); err != nil {
			return err
		}
		s.log.DebugContext(ctx, "skipped embedding for inactive journal",
			slog.String("journal_id", journalID),
			slog.String("status", entry.Status),
		)
		return nil
	}

	chunks := ChunkContent(entry.Content, s.options)
	if len(chunks) == 0 {
		return s.repo.DeleteEmbeddings(ctx, journalID)
	}
	if s.embeddings == nil {
		return fmt.Errorf("%w: embedding provider is not configured", ai.ErrInvalidRequest)
	}

	started := time.Now()
	response, err := s.embeddings.Embed(ctx, ai.EmbeddingRequest{
		Model: s.model,
		Texts: chunkTexts(chunks),
	})
	latency := time.Since(started)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        entry.UserID,
				Operation:     aiusage.OperationEmbedding,
				Provider:      s.provider,
				Model:         s.model,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
				ReferenceID:   &journalID,
			})
		}
		return err
	}
	if len(response.Embeddings) != len(chunks) {
		return fmt.Errorf("%w: expected %d embeddings, got %d", ai.ErrInvalidResponse, len(chunks), len(response.Embeddings))
	}
	for _, vector := range response.Embeddings {
		if len(vector) != s.dimension {
			return fmt.Errorf("%w: got %d dimensions, want %d", ai.ErrDimensionMismatch, len(vector), s.dimension)
		}
	}

	model := response.Model
	if model == "" {
		model = s.model
	}
	if model == "" {
		model = "unknown"
	}

	if s.usage != nil {
		s.usage.Record(ctx, aiusage.Event{
			UserID:      entry.UserID,
			Operation:   aiusage.OperationEmbedding,
			Provider:    s.provider,
			Model:       model,
			InputTokens: response.Usage.InputTokens,
			Latency:     latency,
			Success:     true,
			ReferenceID: &journalID,
		})
	}

	if err := s.repo.ReplaceEmbeddings(ctx, journalID, entry.UserID, chunks, response.Embeddings, model); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "journal embeddings stored",
		slog.String("journal_id", journalID),
		slog.Int("chunks", len(chunks)),
		slog.String("model", model),
		slog.Int("input_tokens", response.Usage.InputTokens),
	)
	return nil
}
