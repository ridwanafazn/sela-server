package insight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/prompts"
)

const (
	journalSourceLimit    = 20
	messageSourceLimit    = 20
	memorySourceLimit     = 10
	contextRuneBudget     = 16000
	maxSourceContentRunes = 3000
	maxEvidence           = 10
	maxEvidenceNoteRunes  = 300
	minEvidenceSources    = 2
	minInsightConfidence  = 0.40
	insightMaxTokens      = 3000
	insightTemperature    = 0.4
	maxInsightTitleRunes  = 200
	maxInsightFieldRunes  = 4000
	maxHypothesisRunes    = 2000
	maxQuestionRunes      = 1000
)

var insightTemplate = template.Must(template.New("insight").Parse(prompts.InsightV1))

var insightSchema = json.RawMessage(`{
  "type": "object",
  "required": ["title", "summary", "observation", "interpretation", "confidence", "reflection_question", "suggested_direction", "evidence"],
  "properties": {
    "title": {"type": "string", "minLength": 1, "maxLength": 200},
    "summary": {"type": "string", "minLength": 1, "maxLength": 4000},
    "observation": {"type": "string", "minLength": 1, "maxLength": 4000},
    "interpretation": {"type": "string", "minLength": 1, "maxLength": 4000},
    "hypothesis": {"type": ["string", "null"], "maxLength": 2000},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "reflection_question": {"type": "string", "minLength": 1, "maxLength": 1000},
    "suggested_direction": {"type": "string", "enum": ["improve", "maintain", "accept"]},
    "evidence": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "type": "object",
        "required": ["source_type", "source_id", "note"],
        "properties": {
          "source_type": {"type": "string", "enum": ["journal", "message", "memory"]},
          "source_id": {"type": "string", "format": "uuid"},
          "note": {"type": "string", "minLength": 1, "maxLength": 300}
        }
      }
    }
  }
}`)

type insightOutput struct {
	Title              string           `json:"title"`
	Summary            string           `json:"summary"`
	Observation        string           `json:"observation"`
	Interpretation     string           `json:"interpretation"`
	Hypothesis         *string          `json:"hypothesis"`
	Confidence         float64          `json:"confidence"`
	ReflectionQuestion string           `json:"reflection_question"`
	SuggestedDirection string           `json:"suggested_direction"`
	Evidence           []outputEvidence `json:"evidence"`
}

type outputEvidence struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Note       string `json:"note"`
}

// GenerationService produces insights from eligible source records.
type GenerationService struct {
	repo  *Repository
	llm   ai.LLMProvider
	model string
	log   *slog.Logger
	now   func() time.Time
}

// NewGenerationService creates the insight generation pipeline.
func NewGenerationService(repo *Repository, llm ai.LLMProvider, model string, log *slog.Logger) *GenerationService {
	if log == nil {
		log = slog.Default()
	}
	return &GenerationService{repo: repo, llm: llm, model: model, log: log, now: time.Now}
}

// Process generates one insight or records why there was not enough
// evidence.
func (s *GenerationService) Process(
	ctx context.Context,
	jobID, userID string,
	payload JobPayload,
) (*GenerationResult, error) {
	existing, err := s.repo.GetByJobID(ctx, jobID)
	if err == nil {
		return insightResult(existing.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	startDate, err := time.Parse("2006-01-02", payload.StartDate)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid insight start date", ai.ErrInvalidRequest)
	}
	endDate, err := time.Parse("2006-01-02", payload.EndDate)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid insight end date", ai.ErrInvalidRequest)
	}

	language, timezone, err := s.repo.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	startBound := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, location).UTC()
	endBound := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1).UTC()

	journals, err := s.repo.EligibleJournals(ctx, userID, startDate, endDate, journalSourceLimit)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.EligibleMessages(ctx, userID, startBound, endBound, messageSourceLimit)
	if err != nil {
		return nil, err
	}
	memories, err := s.repo.EligibleMemories(ctx, userID, memorySourceLimit)
	if err != nil {
		return nil, err
	}

	sources := make([]SourceRecord, 0, len(journals)+len(messages)+len(memories))
	sources = append(sources, journals...)
	sources = append(sources, messages...)
	sources = append(sources, memories...)

	contextText, included := buildInsightContext(sources)
	if len(included) < minEvidenceSources {
		result := insufficientResult("There is not enough eligible context for this period yet. Add a few journal entries, then try again.")
		s.log.InfoContext(ctx, "insight generation skipped",
			slog.String("job_id", jobID),
			slog.String("outcome", result.Outcome),
			slog.Int("sources", len(included)),
		)
		return result, nil
	}

	if s.llm == nil {
		return nil, fmt.Errorf("%w: language provider is not configured", ai.ErrInvalidRequest)
	}

	var prompt strings.Builder
	if err := insightTemplate.Execute(&prompt, struct {
		Language    string
		PeriodStart string
		PeriodEnd   string
		Focus       string
		Context     string
	}{
		Language:    language,
		PeriodStart: payload.StartDate,
		PeriodEnd:   payload.EndDate,
		Focus:       emptyFallback(payload.Focus, "none"),
		Context:     contextText,
	}); err != nil {
		return nil, fmt.Errorf("render insight prompt: %w", err)
	}

	response, err := s.llm.Generate(ctx, ai.GenerateRequest{
		Model:          s.model,
		System:         prompt.String(),
		Messages:       []ai.Message{{Role: ai.RoleUser, Content: "Generate the insight from the provided sources."}},
		Temperature:    insightTemperature,
		MaxTokens:      insightMaxTokens,
		ResponseSchema: insightSchema,
		SessionID:      "insight:" + jobID,
	})
	if err != nil {
		return nil, err
	}

	output, err := parseInsightResponse(response.Content)
	if err != nil {
		return nil, err
	}
	validated, reason, err := validateInsightOutput(output, sources, included)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		result := insufficientResult(reason)
		s.log.InfoContext(ctx, "insight generation below evidence threshold",
			slog.String("job_id", jobID),
			slog.String("outcome", result.Outcome),
			slog.String("reason", reason),
		)
		return result, nil
	}

	now := s.now().UTC()
	insightID, err := ids.New()
	if err != nil {
		return nil, err
	}
	jobIDValue := jobID
	model := response.Model
	if model == "" {
		model = s.model
	}
	promptVersion := prompts.InsightVersion
	periodStart := startDate
	periodEnd := endDate

	record := &Insight{
		ID:                 insightID,
		UserID:             userID,
		JobID:              &jobIDValue,
		Title:              validated.Title,
		Summary:            validated.Summary,
		Observation:        validated.Observation,
		Interpretation:     validated.Interpretation,
		Hypothesis:         validated.Hypothesis,
		Confidence:         validated.Confidence,
		ConfidenceLabel:    confidenceLabel(validated.Confidence),
		ReflectionQuestion: validated.ReflectionQuestion,
		SuggestedDirection: validated.SuggestedDirection,
		PeriodStart:        &periodStart,
		PeriodEnd:          &periodEnd,
		GenerationType:     payload.GenerationType,
		Model:              &model,
		PromptVersion:      &promptVersion,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	evidence := make([]Evidence, 0, len(validated.Evidence))
	for _, item := range validated.Evidence {
		id, err := ids.New()
		if err != nil {
			return nil, err
		}
		recordEvidence := Evidence{
			ID:        id,
			InsightID: insightID,
			Note:      item.Note,
			CreatedAt: now,
		}
		switch item.SourceType {
		case "journal":
			recordEvidence.JournalID = &item.SourceID
		case "message":
			recordEvidence.MessageID = &item.SourceID
		case "memory":
			recordEvidence.MemoryID = &item.SourceID
		}
		evidence = append(evidence, recordEvidence)
	}

	created, err := s.repo.Create(ctx, record, evidence)
	if err != nil {
		return nil, err
	}
	if !created {
		existing, err := s.repo.GetByJobID(ctx, jobID)
		if err != nil {
			return nil, err
		}
		return insightResult(existing.ID), nil
	}

	s.log.InfoContext(ctx, "insight generated",
		slog.String("job_id", jobID),
		slog.String("insight_id", insightID),
		slog.String("model", model),
		slog.Int("input_tokens", response.Usage.InputTokens),
		slog.Int("output_tokens", response.Usage.OutputTokens),
		slog.Int("evidence", len(evidence)),
	)

	return insightResult(insightID), nil
}

func buildInsightContext(sources []SourceRecord) (string, map[string]bool) {
	included := make(map[string]bool, len(sources))
	var builder strings.Builder
	budget := contextRuneBudget

	for _, source := range sources {
		key := sourceKey(source.Type, source.ID)
		if included[key] {
			continue
		}

		header := sourceHeader(source)
		footer := fmt.Sprintf("\n</%s>\n", source.Type)
		available := budget - utf8.RuneCountInString(header) - utf8.RuneCountInString(footer)
		if available < 80 {
			break
		}
		content := truncateRunes(source.Content, minInt(maxSourceContentRunes, available))
		if strings.TrimSpace(content) == "" {
			continue
		}

		builder.WriteString(header)
		builder.WriteString(content)
		builder.WriteString(footer)
		budget -= utf8.RuneCountInString(header) + utf8.RuneCountInString(content) + utf8.RuneCountInString(footer)
		included[key] = true
	}

	return builder.String(), included
}

func sourceHeader(source SourceRecord) string {
	switch source.Type {
	case "journal":
		title := ""
		if source.Title != nil {
			title = *source.Title
		}
		return fmt.Sprintf("<journal id=%q date=%q title=%q>\n", source.ID, source.Date.Format("2006-01-02"), title)
	case "message":
		return fmt.Sprintf("<message id=%q date=%q>\n", source.ID, source.Date.Format("2006-01-02"))
	default:
		category := ""
		if source.Category != nil {
			category = *source.Category
		}
		return fmt.Sprintf("<memory id=%q date=%q category=%q>\n", source.ID, source.Date.Format("2006-01-02"), category)
	}
}

func validateInsightOutput(
	output *insightOutput,
	sources []SourceRecord,
	included map[string]bool,
) (*insightOutput, string, error) {
	if output == nil {
		return nil, "", fmt.Errorf("%w: empty insight output", ai.ErrInvalidResponse)
	}

	output.Title = strings.TrimSpace(output.Title)
	output.Summary = strings.TrimSpace(output.Summary)
	output.Observation = strings.TrimSpace(output.Observation)
	output.Interpretation = strings.TrimSpace(output.Interpretation)
	output.ReflectionQuestion = strings.TrimSpace(output.ReflectionQuestion)

	if output.Title == "" || utf8.RuneCountInString(output.Title) > maxInsightTitleRunes {
		return nil, "", fmt.Errorf("%w: invalid insight title", ai.ErrInvalidResponse)
	}
	for name, value := range map[string]string{
		"summary":        output.Summary,
		"observation":    output.Observation,
		"interpretation": output.Interpretation,
	} {
		if value == "" || utf8.RuneCountInString(value) > maxInsightFieldRunes {
			return nil, "", fmt.Errorf("%w: invalid insight %s", ai.ErrInvalidResponse, name)
		}
	}
	if output.ReflectionQuestion == "" || utf8.RuneCountInString(output.ReflectionQuestion) > maxQuestionRunes {
		return nil, "", fmt.Errorf("%w: invalid reflection question", ai.ErrInvalidResponse)
	}
	switch output.SuggestedDirection {
	case DirectionImprove, DirectionMaintain, DirectionAccept:
	default:
		return nil, "", fmt.Errorf("%w: invalid suggested direction", ai.ErrInvalidResponse)
	}
	if output.Confidence < 0 || output.Confidence > 1 {
		return nil, "", fmt.Errorf("%w: confidence outside bounds", ai.ErrInvalidResponse)
	}

	if output.Hypothesis != nil {
		trimmed := strings.TrimSpace(*output.Hypothesis)
		if trimmed == "" {
			output.Hypothesis = nil
		} else if utf8.RuneCountInString(trimmed) > maxHypothesisRunes {
			return nil, "", fmt.Errorf("%w: hypothesis too long", ai.ErrInvalidResponse)
		} else {
			output.Hypothesis = &trimmed
		}
	}

	sourceByKey := make(map[string]SourceRecord, len(sources))
	for _, source := range sources {
		sourceByKey[sourceKey(source.Type, source.ID)] = source
	}

	seen := make(map[string]bool, len(output.Evidence))
	validEvidence := make([]outputEvidence, 0, len(output.Evidence))
	for _, item := range output.Evidence {
		item.SourceType = strings.TrimSpace(item.SourceType)
		item.Note = strings.TrimSpace(item.Note)
		switch item.SourceType {
		case "journal", "message", "memory":
		default:
			return nil, "", fmt.Errorf("%w: invalid evidence source type", ai.ErrInvalidResponse)
		}
		parsedID, err := parseUUID(item.SourceID)
		if err != nil {
			return nil, "", fmt.Errorf("%w: invalid evidence source id", ai.ErrInvalidResponse)
		}
		item.SourceID = parsedID
		key := sourceKey(item.SourceType, item.SourceID)
		if !included[key] {
			return nil, "", fmt.Errorf("%w: evidence references an unavailable source", ai.ErrInvalidResponse)
		}
		if seen[key] {
			continue
		}
		if item.Note == "" || utf8.RuneCountInString(item.Note) > maxEvidenceNoteRunes {
			return nil, "", fmt.Errorf("%w: invalid evidence note", ai.ErrInvalidResponse)
		}
		source := sourceByKey[key]
		if normalized(item.Note) == normalized(source.Content) {
			return nil, "", fmt.Errorf("%w: evidence note copies the full source", ai.ErrInvalidResponse)
		}
		seen[key] = true
		validEvidence = append(validEvidence, item)
		if len(validEvidence) == maxEvidence {
			break
		}
	}
	output.Evidence = validEvidence

	if len(validEvidence) < minEvidenceSources {
		return nil, "Two or more distinct source records are required for an insight.", nil
	}
	if output.Confidence < minInsightConfidence {
		return nil, "The available evidence does not support a confident enough insight.", nil
	}

	return output, "", nil
}

func parseInsightResponse(content string) (*insightOutput, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty insight output", ai.ErrInvalidResponse)
	}

	var output insightOutput
	if err := json.Unmarshal([]byte(trimmed), &output); err != nil {
		return nil, fmt.Errorf("%w: decode insight output: %v", ai.ErrInvalidResponse, err)
	}
	return &output, nil
}

func confidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.75:
		return ConfidenceStrong
	case confidence >= 0.55:
		return ConfidenceModerate
	default:
		return ConfidenceLimited
	}
}

func insightResult(insightID string) *GenerationResult {
	return &GenerationResult{Outcome: "insight_created", InsightID: &insightID}
}

func insufficientResult(message string) *GenerationResult {
	return &GenerationResult{Outcome: "insufficient_evidence", Message: message}
}

func sourceKey(sourceType, sourceID string) string {
	return sourceType + ":" + sourceID
}

func parseUUID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func normalized(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
