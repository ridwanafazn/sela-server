package memory

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/textquery"
)

const (
	defaultPageLimit    = 20
	maxPageLimit        = 100
	maxRetrievalScanned = 200
	defaultMemoryLimit  = 5
	maxMemoryLimit      = 10
	maxCategoryLength   = 100
)

// Service implements memory review and Companion retrieval.
type Service struct {
	repo *Repository
	log  *slog.Logger
	now  func() time.Time
}

// NewService creates the memory service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, log: log, now: time.Now}
}

// List returns a page of memories.
func (s *Service) List(ctx context.Context, userID string, filter ListFilter) (*Page, error) {
	if !validStatusFilter(filter.Status) {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "Status must be all, active, candidate, rejected, or superseded.",
		})
	}

	category := strings.TrimSpace(filter.Category)
	if utf8.RuneCountInString(category) > maxCategoryLength {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "category",
			Code:    "TOO_LONG",
			Message: "Category must be at most 100 characters.",
		})
	}

	limit := clampLimit(filter.Limit)

	var (
		afterUpdatedAt *time.Time
		afterID        string
	)
	if filter.Cursor != "" {
		updatedAt, id, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "cursor",
				Code:    "INVALID",
				Message: "The cursor is invalid.",
			})
		}
		afterUpdatedAt = &updatedAt
		afterID = id
	}

	memories, err := s.repo.List(ctx, userID, filter.Status, category, afterUpdatedAt, afterID, limit)
	if err != nil {
		return nil, err
	}

	hasMore := len(memories) > limit
	if hasMore {
		memories = memories[:limit]
	}

	items := make([]View, 0, len(memories))
	for index := range memories {
		items = append(items, newView(&memories[index]))
	}

	var nextCursor *string
	if hasMore && len(memories) > 0 {
		last := memories[len(memories)-1]
		value := encodeCursor(last.UpdatedAt, last.ID)
		nextCursor = &value
	}

	return &Page{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// Get returns one owned memory with its sources.
func (s *Service) Get(ctx context.Context, userID, memoryID string) (*Detail, error) {
	memory, err := s.repo.Get(ctx, userID, memoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, memoryNotFound()
	}
	if err != nil {
		return nil, err
	}

	sources, err := s.repo.Sources(ctx, memory.ID)
	if err != nil {
		return nil, err
	}

	detail := &Detail{View: newView(memory), Sources: sources}
	if detail.Sources == nil {
		detail.Sources = []SourceView{}
	}
	return detail, nil
}

// Update corrects a memory's content or category.
func (s *Service) Update(ctx context.Context, userID, memoryID string, input UpdateInput) (*Detail, error) {
	if input.Content == nil && !input.CategorySet {
		return s.Get(ctx, userID, memoryID)
	}

	var content *string
	if input.Content != nil {
		normalized, err := validateContent(*input.Content)
		if err != nil {
			return nil, err
		}
		content = &normalized
	}

	var category *string
	if input.CategorySet && input.Category != nil {
		normalized, err := validateCategory(*input.Category)
		if err != nil {
			return nil, err
		}
		category = normalized
	}

	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		memory, err := s.repo.GetForUpdate(tx, userID, memoryID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return memoryNotFound()
		}
		if err != nil {
			return err
		}

		if content != nil {
			memory.Content = *content
		}
		if input.CategorySet {
			memory.Category = category
		}
		memory.UpdatedAt = s.now().UTC()

		return s.repo.Save(tx, memory)
	})
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, userID, memoryID)
}

// Feedback confirms, corrects, or rejects a memory.
func (s *Service) Feedback(ctx context.Context, userID, memoryID string, input FeedbackInput) (*Detail, error) {
	var corrected *string
	switch input.Action {
	case "confirm", "reject":
	case "correct":
		if input.CorrectedContent == nil {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "corrected_content",
				Code:    "REQUIRED",
				Message: "Corrected content is required when action is correct.",
			})
		}
		normalized, err := validateContent(*input.CorrectedContent)
		if err != nil {
			return nil, err
		}
		corrected = &normalized
	default:
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "action",
			Code:    "INVALID_VALUE",
			Message: "Action must be confirm, correct, or reject.",
		})
	}

	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		memory, err := s.repo.GetForUpdate(tx, userID, memoryID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return memoryNotFound()
		}
		if err != nil {
			return err
		}

		now := s.now().UTC()
		switch input.Action {
		case "confirm":
			memory.Status = StatusActive
			memory.LastConfirmedAt = &now
		case "correct":
			memory.Content = *corrected
			memory.Status = StatusActive
			memory.LastConfirmedAt = &now
		case "reject":
			memory.Status = StatusRejected
		}
		memory.UpdatedAt = now

		return s.repo.Save(tx, memory)
	})
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, userID, memoryID)
}

// Delete soft-deletes a memory.
func (s *Service) Delete(ctx context.Context, userID, memoryID string) error {
	return s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		memory, err := s.repo.GetForUpdate(tx, userID, memoryID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return memoryNotFound()
		}
		if err != nil {
			return err
		}
		return s.repo.SoftDelete(tx, memory, s.now().UTC())
	})
}

// Retrieve returns memories relevant to a Companion message. Confirmed
// memories rank above unconfirmed candidates and excluded statuses never
// appear.
func (s *Service) Retrieve(ctx context.Context, userID, query string, limit int) ([]RetrievedMemory, error) {
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	if limit > maxMemoryLimit {
		limit = maxMemoryLimit
	}

	terms := textquery.Terms(query, textquery.MaxTerms)
	if len(terms) == 0 {
		return nil, nil
	}

	candidates, err := s.repo.RetrievalCandidates(ctx, userID, maxRetrievalScanned)
	if err != nil {
		return nil, err
	}

	retrieved := rankMemories(candidates, terms, limit)

	s.log.DebugContext(ctx, "memory retrieval completed",
		slog.Int("candidates", len(candidates)),
		slog.Int("returned", len(retrieved)),
	)

	return retrieved, nil
}

func rankMemories(candidates []RetrievedMemory, terms []string, limit int) []RetrievedMemory {
	type scored struct {
		memory RetrievedMemory
		score  float64
	}

	results := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Status != StatusActive && candidate.Status != StatusCandidate {
			continue
		}

		tokens := tokenSet(candidate.Content)
		if candidate.Category != nil {
			for token := range tokenSet(*candidate.Category) {
				tokens[token] = true
			}
		}

		matches := 0
		for _, term := range terms {
			if tokens[term] {
				matches++
			}
		}
		if matches == 0 {
			continue
		}

		score := float64(matches)/float64(len(terms)) + candidate.Confidence*0.1
		if candidate.Status == StatusActive {
			score += 0.15
		} else {
			score -= 0.05
		}
		results = append(results, scored{memory: candidate, score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		if !results[i].memory.UpdatedAt.Equal(results[j].memory.UpdatedAt) {
			return results[i].memory.UpdatedAt.After(results[j].memory.UpdatedAt)
		}
		return results[i].memory.ID > results[j].memory.ID
	})

	if len(results) > limit {
		results = results[:limit]
	}

	retrieved := make([]RetrievedMemory, 0, len(results))
	for _, result := range results {
		retrieved = append(retrieved, result.memory)
	}
	return retrieved
}

func validateContent(content string) (string, *apierrors.Error) {
	normalized := strings.TrimSpace(content)
	runeCount := utf8.RuneCountInString(normalized)
	if runeCount == 0 {
		return "", apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "content",
			Code:    "REQUIRED",
			Message: "Content must not be empty.",
		})
	}
	if runeCount > maxMemoryContentRunes {
		return "", apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "content",
			Code:    "TOO_LONG",
			Message: "Content must be at most 500 characters.",
		})
	}
	return normalized, nil
}

func validateCategory(category string) (*string, *apierrors.Error) {
	normalized := normalizeCategory(category)
	if normalized == nil {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "category",
			Code:    "INVALID_VALUE",
			Message: "Category must contain a letter or digit.",
		})
	}
	if utf8.RuneCountInString(*normalized) > maxCategoryLength {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "category",
			Code:    "TOO_LONG",
			Message: "Category must be at most 100 characters.",
		})
	}
	return normalized, nil
}

func tokenSet(text string) map[string]bool {
	tokens := make(map[string]bool)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value)
	}) {
		if len([]rune(token)) >= 3 {
			tokens[token] = true
		}
	}
	return tokens
}

func validStatusFilter(status string) bool {
	switch status {
	case "", "all", StatusActive, StatusCandidate, StatusRejected, StatusSuperseded:
		return true
	default:
		return false
	}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > maxPageLimit {
		return maxPageLimit
	}
	return limit
}

func memoryNotFound() *apierrors.Error {
	return apierrors.NotFound("The memory was not found.")
}
