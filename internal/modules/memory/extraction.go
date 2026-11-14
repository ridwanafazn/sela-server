package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/prompts"
)

const (
	maxCandidatesPerSource = 10
	maxMemoryContentRunes  = 500
	minMemoryContentRunes  = 3
	maxCategoryRunes       = 100
	extractionMaxTokens    = 3000
	extractionTemperature  = 0.2
)

var extractionSchema = json.RawMessage(`{
  "type": "object",
  "required": ["memories"],
  "properties": {
    "memories": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "type": "object",
        "required": ["category", "content", "confidence", "certainty_type", "source_ids", "sensitivity", "conflicts_with"],
        "properties": {
          "category": {"type": "string", "maxLength": 100},
          "content": {"type": "string", "minLength": 1, "maxLength": 500},
          "confidence": {"type": "number", "minimum": 0, "maximum": 1},
          "certainty_type": {"type": "string", "enum": ["explicit", "inferred"]},
          "source_ids": {"type": "array", "minItems": 1, "items": {"type": "string", "format": "uuid"}},
          "sensitivity": {"type": "string", "enum": ["normal", "sensitive"]},
          "conflicts_with": {"type": "array", "items": {"type": "string", "format": "uuid"}}
        }
      }
    }
  }
}`)

type extractionResponse struct {
	Memories []extractedMemory `json:"memories"`
}

type extractedMemory struct {
	Category      string   `json:"category"`
	Content       string   `json:"content"`
	Confidence    *float64 `json:"confidence"`
	CertaintyType string   `json:"certainty_type"`
	SourceIDs     []string `json:"source_ids"`
	Sensitivity   string   `json:"sensitivity"`
	ConflictsWith []string `json:"conflicts_with"`
}

type validatedCandidate struct {
	Content    string
	Category   *string
	Confidence float64
}

// ExtractionRepository is the persistence required by the extraction worker.
type ExtractionRepository interface {
	JournalForExtraction(ctx context.Context, journalID string) (*JournalSource, error)
	EligibleJournalIDs(ctx context.Context, userID string, journalIDs []string) (map[string]bool, error)
	OwnedMemoryIDs(ctx context.Context, userID string, memoryIDs []string) (map[string]bool, error)
	FindByNormalizedContent(ctx context.Context, userID, content string) (*Memory, error)
	AddJournalSource(ctx context.Context, memoryID, journalID string, now time.Time) error
	Create(ctx context.Context, memory *Memory, journalID string) error
}

// ExtractionService turns journal entries into candidate memories.
type ExtractionService struct {
	repo  ExtractionRepository
	llm   ai.LLMProvider
	model string
	log   *slog.Logger
	now   func() time.Time
}

// NewExtractionService creates the memory extraction pipeline.
func NewExtractionService(repo ExtractionRepository, llm ai.LLMProvider, model string, log *slog.Logger) *ExtractionService {
	if log == nil {
		log = slog.Default()
	}
	return &ExtractionService{
		repo:  repo,
		llm:   llm,
		model: model,
		log:   log,
		now:   time.Now,
	}
}

// ProcessJournal extracts candidate memories from one active journal.
func (s *ExtractionService) ProcessJournal(ctx context.Context, journalID string) error {
	entry, err := s.repo.JournalForExtraction(ctx, journalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if entry.DeletedAt != nil || entry.Status != "active" {
		s.log.DebugContext(ctx, "skipped memory extraction for ineligible journal",
			slog.String("journal_id", journalID),
			slog.String("status", entry.Status),
		)
		return nil
	}
	if s.llm == nil {
		return fmt.Errorf("%w: language provider is not configured", ai.ErrInvalidRequest)
	}

	response, err := s.llm.Generate(ctx, ai.GenerateRequest{
		Model:          s.model,
		System:         prompts.MemoryExtractionV1,
		Messages:       []ai.Message{{Role: ai.RoleUser, Content: extractionInput(entry)}},
		Temperature:    extractionTemperature,
		MaxTokens:      extractionMaxTokens,
		ResponseSchema: extractionSchema,
		SessionID:      "memory-extraction:" + entry.ID,
	})
	if err != nil {
		return err
	}

	parsed, err := parseExtractionResponse(response.Content)
	if err != nil {
		return err
	}
	if len(parsed) > maxCandidatesPerSource {
		parsed = parsed[:maxCandidatesPerSource]
	}

	now := s.now().UTC()
	seen := make(map[string]bool, len(parsed))
	valid := make([]validatedCandidate, 0, len(parsed))

	var sensitiveSkipped, invalidSkipped int
	for _, raw := range parsed {
		candidate, accepted, sensitive, err := s.validateCandidate(ctx, entry, raw)
		if err != nil {
			return err
		}
		if sensitive {
			sensitiveSkipped++
			continue
		}
		if !accepted {
			invalidSkipped++
			continue
		}

		key := normalizedContent(candidate.Content)
		if seen[key] {
			invalidSkipped++
			continue
		}
		seen[key] = true
		valid = append(valid, *candidate)
	}

	var created, merged, skipped int
	for _, candidate := range valid {
		existing, err := s.repo.FindByNormalizedContent(ctx, entry.UserID, normalizedContent(candidate.Content))
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			if existing.Status == StatusRejected ||
				existing.Status == StatusSuperseded ||
				existing.Status == StatusDeleted {
				skipped++
				continue
			}
			if err := s.repo.AddJournalSource(ctx, existing.ID, entry.ID, now); err != nil {
				return err
			}
			merged++
			continue
		}

		id, err := ids.New()
		if err != nil {
			return err
		}
		memory := &Memory{
			ID:          id,
			UserID:      entry.UserID,
			Content:     candidate.Content,
			Category:    candidate.Category,
			Status:      StatusCandidate,
			Confidence:  &candidate.Confidence,
			SourceCount: 1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.Create(ctx, memory, entry.ID); err != nil {
			return err
		}
		created++
	}

	s.log.InfoContext(ctx, "journal memory extraction completed",
		slog.String("journal_id", entry.ID),
		slog.String("prompt_version", prompts.MemoryExtractionVersion),
		slog.String("model", response.Model),
		slog.Int("input_tokens", response.Usage.InputTokens),
		slog.Int("output_tokens", response.Usage.OutputTokens),
		slog.Int("model_candidates", len(parsed)),
		slog.Int("created", created),
		slog.Int("merged", merged),
		slog.Int("skipped", skipped+invalidSkipped),
		slog.Int("sensitive_skipped", sensitiveSkipped),
	)

	return nil
}

func (s *ExtractionService) validateCandidate(
	ctx context.Context,
	entry *JournalSource,
	raw extractedMemory,
) (*validatedCandidate, bool, bool, error) {
	if raw.Sensitivity == "sensitive" {
		return nil, false, true, nil
	}
	if raw.Sensitivity != "normal" {
		return nil, false, false, nil
	}
	if raw.CertaintyType != "explicit" && raw.CertaintyType != "inferred" {
		return nil, false, false, nil
	}
	if raw.Confidence == nil || *raw.Confidence < 0 || *raw.Confidence > 1 {
		return nil, false, false, nil
	}

	content := strings.TrimSpace(raw.Content)
	runeCount := utf8.RuneCountInString(content)
	if runeCount < minMemoryContentRunes || runeCount > maxMemoryContentRunes {
		return nil, false, false, nil
	}
	if normalizedContent(content) == normalizedContent(entry.Content) {
		return nil, false, false, nil
	}

	sourceIDs := uniqueIDs(raw.SourceIDs)
	if len(sourceIDs) != 1 {
		return nil, false, false, nil
	}
	sourceID, err := uuid.Parse(sourceIDs[0])
	if err != nil || sourceID.String() != entry.ID {
		return nil, false, false, nil
	}
	sourceIDs = []string{sourceID.String()}

	eligible, err := s.repo.EligibleJournalIDs(ctx, entry.UserID, sourceIDs)
	if err != nil {
		return nil, false, false, err
	}
	if !eligible[entry.ID] {
		return nil, false, false, nil
	}

	conflicts := uniqueIDs(raw.ConflictsWith)
	canonicalConflicts := make([]string, 0, len(conflicts))
	for _, id := range conflicts {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return nil, false, false, nil
		}
		canonicalConflicts = append(canonicalConflicts, parsed.String())
	}
	if len(canonicalConflicts) > 0 {
		owned, err := s.repo.OwnedMemoryIDs(ctx, entry.UserID, canonicalConflicts)
		if err != nil {
			return nil, false, false, err
		}
		if len(owned) != len(canonicalConflicts) {
			return nil, false, false, nil
		}
	}

	confidence := *raw.Confidence
	return &validatedCandidate{
		Content:    content,
		Category:   normalizeCategory(raw.Category),
		Confidence: confidence,
	}, true, false, nil
}

func extractionInput(entry *JournalSource) string {
	title := ""
	if entry.Title != nil {
		title = *entry.Title
	}
	return fmt.Sprintf(
		"<journal id=%q date=%q title=%q>\n%s\n</journal>",
		entry.ID,
		entry.EntryDate.Format("2006-01-02"),
		title,
		entry.Content,
	)
}

func parseExtractionResponse(content string) ([]extractedMemory, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty extraction output", ai.ErrInvalidResponse)
	}

	var response extractionResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return nil, fmt.Errorf("%w: decode extraction output: %v", ai.ErrInvalidResponse, err)
	}
	return response.Memories, nil
}

func uniqueIDs(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func normalizeCategory(raw string) *string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return nil
	}

	var builder strings.Builder
	lastSeparator := false
	for _, value := range trimmed {
		switch {
		case unicode.IsLetter(value) || unicode.IsDigit(value):
			builder.WriteRune(value)
			lastSeparator = false
		case value == '_' || value == '-' || unicode.IsSpace(value):
			if builder.Len() > 0 && !lastSeparator {
				builder.WriteByte('_')
				lastSeparator = true
			}
		}
	}

	normalized := strings.Trim(builder.String(), "_")
	if normalized == "" {
		return nil
	}
	if utf8.RuneCountInString(normalized) > maxCategoryRunes {
		normalized = string([]rune(normalized)[:maxCategoryRunes])
		normalized = strings.Trim(normalized, "_")
	}
	if normalized == "" {
		return nil
	}
	return &normalized
}

func normalizedContent(content string) string {
	return strings.ToLower(strings.Join(strings.Fields(content), " "))
}
