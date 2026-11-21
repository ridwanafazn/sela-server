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
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/prompts"
)

const (
	sourceTypeJournal = "journal"
	sourceTypeMessage = "message"

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

type extractionSource struct {
	Type       string
	ID         string
	UserID     string
	Content    string
	Title      *string
	OccurredAt time.Time
}

type validatedCandidate struct {
	Content       string
	Category      *string
	Sensitivity   string
	Confidence    float64
	ConflictsWith []string
}

// ExtractionRepository is the persistence required by the extraction worker.
type ExtractionRepository interface {
	JournalForExtraction(ctx context.Context, journalID string) (*JournalSource, error)
	MessageForExtraction(ctx context.Context, messageID string) (*MessageSource, error)
	EligibleJournalIDs(ctx context.Context, userID string, journalIDs []string) (map[string]bool, error)
	EligibleMessageIDs(ctx context.Context, userID string, messageIDs []string) (map[string]bool, error)
	OwnedMemoryIDs(ctx context.Context, userID string, memoryIDs []string) (map[string]bool, error)
	HasSensitiveMemoryConsent(ctx context.Context, userID string) (bool, error)
	FindByNormalizedContent(ctx context.Context, userID, content string) (*Memory, error)
	AddSource(ctx context.Context, memoryID string, source SourceRef, now time.Time) error
	MarkSensitive(ctx context.Context, memoryID string, now time.Time) error
	AddConflicts(ctx context.Context, memoryID string, conflicts []string, now time.Time) error
	Create(ctx context.Context, memory *Memory, source SourceRef, conflicts []string) error
}

// ExtractionService turns source records into candidate memories.
type ExtractionService struct {
	repo     ExtractionRepository
	llm      ai.LLMProvider
	model    string
	provider string
	usage    *aiusage.Service
	privacy  PrivacyReader
	log      *slog.Logger
	now      func() time.Time
}

// PrivacyReader reports whether automatic memory extraction is enabled.
type PrivacyReader interface {
	MemoryExtractionEnabled(ctx context.Context, userID string) (bool, error)
}

// WithPrivacy configures memory-extraction enforcement.
func (s *ExtractionService) WithPrivacy(reader PrivacyReader) *ExtractionService {
	s.privacy = reader
	return s
}

// NewExtractionService creates the memory extraction pipeline.
func NewExtractionService(
	repo ExtractionRepository,
	llm ai.LLMProvider,
	model, provider string,
	usage *aiusage.Service,
	log *slog.Logger,
) *ExtractionService {
	if log == nil {
		log = slog.Default()
	}
	return &ExtractionService{
		repo:     repo,
		llm:      llm,
		model:    model,
		provider: provider,
		usage:    usage,
		log:      log,
		now:      time.Now,
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
	if !s.extractionEnabled(ctx, entry.UserID, "journal", journalID) {
		return nil
	}

	return s.process(ctx, &extractionSource{
		Type:       sourceTypeJournal,
		ID:         entry.ID,
		UserID:     entry.UserID,
		Content:    entry.Content,
		Title:      entry.Title,
		OccurredAt: entry.EntryDate,
	})
}

// ProcessMessage extracts candidate memories from one user message in an
// active conversation.
func (s *ExtractionService) ProcessMessage(ctx context.Context, messageID string) error {
	message, err := s.repo.MessageForExtraction(ctx, messageID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if message.Role != "user" || message.ConversationDeletedAt != nil {
		s.log.DebugContext(ctx, "skipped memory extraction for ineligible message",
			slog.String("message_id", messageID),
			slog.String("role", message.Role),
		)
		return nil
	}
	if !s.extractionEnabled(ctx, message.UserID, "message", messageID) {
		return nil
	}

	return s.process(ctx, &extractionSource{
		Type:       sourceTypeMessage,
		ID:         message.ID,
		UserID:     message.UserID,
		Content:    message.Content,
		OccurredAt: message.CreatedAt,
	})
}

func (s *ExtractionService) extractionEnabled(ctx context.Context, userID, sourceType, sourceID string) bool {
	if s.privacy == nil {
		return true
	}
	enabled, err := s.privacy.MemoryExtractionEnabled(ctx, userID)
	if err != nil {
		s.log.WarnContext(ctx, "memory extraction privacy lookup failed",
			slog.String("source_type", sourceType),
			slog.String("source_id", sourceID),
			slog.String("error", err.Error()),
		)
		return true
	}
	if !enabled {
		s.log.DebugContext(ctx, "skipped memory extraction by privacy preference",
			slog.String("source_type", sourceType),
			slog.String("source_id", sourceID),
		)
	}
	return enabled
}

func (s *ExtractionService) process(ctx context.Context, source *extractionSource) error {
	if s.llm == nil {
		return fmt.Errorf("%w: language provider is not configured", ai.ErrInvalidRequest)
	}
	if s.usage != nil {
		exhausted, err := s.usage.Exhausted(ctx, source.UserID)
		if err != nil {
			s.log.WarnContext(ctx, "memory extraction budget check failed", slog.String("error", err.Error()))
		} else if exhausted {
			s.log.WarnContext(ctx, "memory extraction skipped because the ai budget is exhausted",
				slog.String("source_type", source.Type),
				slog.String("source_id", source.ID),
			)
			return nil
		}
	}

	operation := aiusage.OperationMemoryJournal
	if source.Type == sourceTypeMessage {
		operation = aiusage.OperationMemoryMessage
	}
	referenceID := source.ID
	started := time.Now()
	response, err := s.llm.Generate(ctx, ai.GenerateRequest{
		Model:          s.model,
		System:         prompts.MemoryExtractionV2,
		Messages:       []ai.Message{{Role: ai.RoleUser, Content: extractionInput(source)}},
		Temperature:    extractionTemperature,
		MaxTokens:      extractionMaxTokens,
		ResponseSchema: extractionSchema,
		SessionID:      "memory-extraction:" + source.Type + ":" + source.ID,
	})
	latency := time.Since(started)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        source.UserID,
				Operation:     operation,
				Provider:      s.provider,
				Model:         s.model,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
				ReferenceID:   &referenceID,
			})
		}
		return err
	}

	parsed, err := parseExtractionResponse(response.Content)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        source.UserID,
				Operation:     operation,
				Provider:      s.provider,
				Model:         response.Model,
				InputTokens:   response.Usage.InputTokens,
				OutputTokens:  response.Usage.OutputTokens,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
				ReferenceID:   &referenceID,
			})
		}
		return err
	}
	if s.usage != nil {
		s.usage.Record(ctx, aiusage.Event{
			UserID:       source.UserID,
			Operation:    operation,
			Provider:     s.provider,
			Model:        response.Model,
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			Latency:      latency,
			Success:      true,
			ReferenceID:  &referenceID,
		})
	}
	if len(parsed) > maxCandidatesPerSource {
		parsed = parsed[:maxCandidatesPerSource]
	}

	sensitiveConsent, err := s.repo.HasSensitiveMemoryConsent(ctx, source.UserID)
	if err != nil {
		return err
	}

	now := s.now().UTC()
	seen := make(map[string]bool, len(parsed))
	valid := make([]validatedCandidate, 0, len(parsed))

	var sensitiveSkipped, sensitiveStored, invalidSkipped int
	for _, raw := range parsed {
		candidate, accepted, sensitive, err := s.validateCandidate(ctx, source, raw, sensitiveConsent)
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
		if candidate.Sensitivity == SensitivitySensitive {
			sensitiveStored++
		}

		key := normalizedContent(candidate.Content)
		if seen[key] {
			invalidSkipped++
			continue
		}
		seen[key] = true
		valid = append(valid, *candidate)
	}

	sourceRef := source.reference()
	var created, merged, skipped int
	for _, candidate := range valid {
		existing, err := s.repo.FindByNormalizedContent(ctx, source.UserID, normalizedContent(candidate.Content))
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
			if err := s.repo.AddSource(ctx, existing.ID, sourceRef, now); err != nil {
				return err
			}
			if candidate.Sensitivity == SensitivitySensitive {
				if err := s.repo.MarkSensitive(ctx, existing.ID, now); err != nil {
					return err
				}
			}
			if err := s.repo.AddConflicts(ctx, existing.ID, candidate.ConflictsWith, now); err != nil {
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
			UserID:      source.UserID,
			Content:     candidate.Content,
			Category:    candidate.Category,
			Sensitivity: candidate.Sensitivity,
			Status:      StatusCandidate,
			Confidence:  &candidate.Confidence,
			SourceCount: 1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.Create(ctx, memory, sourceRef, candidate.ConflictsWith); err != nil {
			return err
		}
		created++
	}

	s.log.InfoContext(ctx, "memory extraction completed",
		slog.String("source_type", source.Type),
		slog.String("source_id", source.ID),
		slog.String("prompt_version", prompts.MemoryExtractionVersion),
		slog.String("model", response.Model),
		slog.Int("input_tokens", response.Usage.InputTokens),
		slog.Int("output_tokens", response.Usage.OutputTokens),
		slog.Int("model_candidates", len(parsed)),
		slog.Int("created", created),
		slog.Int("merged", merged),
		slog.Int("skipped", skipped+invalidSkipped),
		slog.Int("sensitive_skipped", sensitiveSkipped),
		slog.Int("sensitive_stored", sensitiveStored),
	)

	return nil
}

func (s *ExtractionService) validateCandidate(
	ctx context.Context,
	source *extractionSource,
	raw extractedMemory,
	sensitiveConsent bool,
) (*validatedCandidate, bool, bool, error) {
	sensitivity := SensitivityNormal
	switch raw.Sensitivity {
	case SensitivitySensitive:
		if !sensitiveConsent {
			return nil, false, true, nil
		}
		sensitivity = SensitivitySensitive
	case SensitivityNormal:
	default:
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
	if normalizedContent(content) == normalizedContent(source.Content) {
		return nil, false, false, nil
	}

	sourceIDs := uniqueIDs(raw.SourceIDs)
	if len(sourceIDs) != 1 {
		return nil, false, false, nil
	}
	sourceID, err := uuid.Parse(sourceIDs[0])
	if err != nil || sourceID.String() != source.ID {
		return nil, false, false, nil
	}

	eligible, err := s.sourceEligible(ctx, source.UserID, source.Type, source.ID)
	if err != nil {
		return nil, false, false, err
	}
	if !eligible {
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
		owned, err := s.repo.OwnedMemoryIDs(ctx, source.UserID, canonicalConflicts)
		if err != nil {
			return nil, false, false, err
		}
		if len(owned) != len(canonicalConflicts) {
			return nil, false, false, nil
		}
	}

	confidence := *raw.Confidence
	return &validatedCandidate{
		Content:       content,
		Category:      normalizeCategory(raw.Category),
		Sensitivity:   sensitivity,
		Confidence:    confidence,
		ConflictsWith: canonicalConflicts,
	}, true, false, nil
}

func (s *ExtractionService) sourceEligible(ctx context.Context, userID, sourceType, sourceID string) (bool, error) {
	switch sourceType {
	case sourceTypeJournal:
		eligible, err := s.repo.EligibleJournalIDs(ctx, userID, []string{sourceID})
		if err != nil {
			return false, err
		}
		return eligible[sourceID], nil
	case sourceTypeMessage:
		eligible, err := s.repo.EligibleMessageIDs(ctx, userID, []string{sourceID})
		if err != nil {
			return false, err
		}
		return eligible[sourceID], nil
	default:
		return false, nil
	}
}

func (s *extractionSource) reference() SourceRef {
	switch s.Type {
	case sourceTypeJournal:
		id := s.ID
		return SourceRef{JournalID: &id}
	case sourceTypeMessage:
		id := s.ID
		return SourceRef{MessageID: &id}
	default:
		return SourceRef{}
	}
}

func extractionInput(source *extractionSource) string {
	switch source.Type {
	case sourceTypeMessage:
		return fmt.Sprintf(
			"<message id=%q date=%q role=\"user\">\n%s\n</message>",
			source.ID,
			source.OccurredAt.Format("2006-01-02"),
			source.Content,
		)
	default:
		title := ""
		if source.Title != nil {
			title = *source.Title
		}
		return fmt.Sprintf(
			"<journal id=%q date=%q title=%q>\n%s\n</journal>",
			source.ID,
			source.OccurredAt.Format("2006-01-02"),
			title,
			source.Content,
		)
	}
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
