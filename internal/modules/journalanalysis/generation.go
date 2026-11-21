package journalanalysis

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

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/prompts"
)

const (
	minAnalysisContentRunes = 80
	minAnalysisConfidence   = 0.40
	maxAnalysisContentRunes = 12000
	maxAnalysisFieldRunes   = 4000
	maxAnalysisHypothesis   = 2000
	maxAnalysisQuestion     = 1000
	maxAnalysisEvidenceNote = 300
	analysisMaxTokens       = 5000
	analysisTemperature     = 0.3
)

var analysisTemplate = template.Must(template.New("journal_analysis").Parse(prompts.JournalAnalysisV1))

var analysisSchema = json.RawMessage(`{
  "type": "object",
  "required": ["summary", "observation", "interpretation", "confidence", "reflection_question", "suggested_direction", "evidence_note"],
  "properties": {
    "summary": {"type": "string", "minLength": 1, "maxLength": 4000},
    "observation": {"type": "string", "minLength": 1, "maxLength": 4000},
    "interpretation": {"type": "string", "minLength": 1, "maxLength": 4000},
    "hypothesis": {"type": ["string", "null"], "maxLength": 2000},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "reflection_question": {"type": "string", "minLength": 1, "maxLength": 1000},
    "suggested_direction": {"type": "string", "enum": ["improve", "maintain", "accept"]},
    "evidence_note": {"type": "string", "minLength": 1, "maxLength": 300}
  }
}`)

type analysisOutput struct {
	Summary            string  `json:"summary"`
	Observation        string  `json:"observation"`
	Interpretation     string  `json:"interpretation"`
	Hypothesis         *string `json:"hypothesis"`
	Confidence         float64 `json:"confidence"`
	ReflectionQuestion string  `json:"reflection_question"`
	SuggestedDirection string  `json:"suggested_direction"`
	EvidenceNote       string  `json:"evidence_note"`
}

// GenerationService produces one analysis from one journal entry.
type GenerationService struct {
	repo     *Repository
	journals JournalReader
	llm      ai.LLMProvider
	model    string
	provider string
	usage    *aiusage.Service
	log      *slog.Logger
	now      func() time.Time
}

// NewGenerationService creates the journal analysis pipeline.
func NewGenerationService(
	repo *Repository,
	journals JournalReader,
	llm ai.LLMProvider,
	model, provider string,
	usage *aiusage.Service,
	log *slog.Logger,
) *GenerationService {
	if log == nil {
		log = slog.Default()
	}
	return &GenerationService{
		repo:     repo,
		journals: journals,
		llm:      llm,
		model:    model,
		provider: provider,
		usage:    usage,
		log:      log,
		now:      time.Now,
	}
}

// Process generates one analysis or records why it was not created.
func (s *GenerationService) Process(
	ctx context.Context,
	jobID, userID string,
	payload JobPayload,
) (*GenerationResult, error) {
	existing, err := s.repo.GetByJobID(ctx, jobID)
	if err == nil {
		return analysisResult(existing.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	entry, err := s.journals.Get(ctx, userID, payload.JournalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return journalChangedResult("The journal was deleted. No analysis was created."), nil
	}
	if err != nil {
		return nil, err
	}
	if entry.Version != payload.JournalVersion || entry.Status != journal.StatusActive {
		return journalChangedResult("The journal changed after the request. Request a new analysis."), nil
	}

	content := strings.TrimSpace(entry.Content)
	if utf8.RuneCountInString(content) < minAnalysisContentRunes {
		return insufficientResult("This journal entry is too short for a meaningful analysis. Add more detail and try again."), nil
	}

	existingVersion, err := s.repo.GetByJournalVersion(ctx, userID, entry.ID, entry.Version)
	if err == nil {
		return analysisResult(existingVersion.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if s.llm == nil {
		return nil, fmt.Errorf("%w: language provider is not configured", ai.ErrInvalidRequest)
	}
	if s.usage != nil {
		exhausted, err := s.usage.Exhausted(ctx, userID)
		if err != nil {
			s.log.WarnContext(ctx, "journal analysis budget check failed",
				slog.String("job_id", jobID),
				slog.String("error", err.Error()),
			)
		} else if exhausted {
			return &GenerationResult{
				Outcome: OutcomeBudgetExhausted,
				Message: "The AI budget for this period is exhausted. Analysis will resume when the budget resets.",
			}, nil
		}
	}

	language, err := s.repo.ProfileLanguage(ctx, userID)
	if err != nil {
		return nil, err
	}

	title := "none"
	if entry.Title != nil && strings.TrimSpace(*entry.Title) != "" {
		title = strings.TrimSpace(*entry.Title)
	}

	var prompt strings.Builder
	if err := analysisTemplate.Execute(&prompt, struct {
		Language  string
		EntryDate string
		Title     string
		Content   string
	}{
		Language:  language,
		EntryDate: entry.EntryDate.Format("2006-01-02"),
		Title:     title,
		Content:   truncateRunes(content, maxAnalysisContentRunes),
	}); err != nil {
		return nil, fmt.Errorf("render journal analysis prompt: %w", err)
	}

	started := time.Now()
	response, err := s.llm.Generate(ctx, ai.GenerateRequest{
		Model:          s.model,
		System:         prompt.String(),
		Messages:       []ai.Message{{Role: ai.RoleUser, Content: "Analyze the journal entry provided in the system prompt."}},
		Temperature:    analysisTemperature,
		MaxTokens:      analysisMaxTokens,
		ResponseSchema: analysisSchema,
		SessionID:      "journal-analysis:" + jobID,
	})
	latency := time.Since(started)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        userID,
				Operation:     aiusage.OperationJournalAnalysis,
				Provider:      s.provider,
				Model:         s.model,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
			})
		}
		return nil, err
	}

	output, err := parseAnalysisResponse(response.Content)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        userID,
				Operation:     aiusage.OperationJournalAnalysis,
				Provider:      s.provider,
				Model:         response.Model,
				InputTokens:   response.Usage.InputTokens,
				OutputTokens:  response.Usage.OutputTokens,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
			})
		}
		return nil, err
	}

	validated, reason, err := validateAnalysisOutput(output, content)
	if err != nil {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:        userID,
				Operation:     aiusage.OperationJournalAnalysis,
				Provider:      s.provider,
				Model:         response.Model,
				InputTokens:   response.Usage.InputTokens,
				OutputTokens:  response.Usage.OutputTokens,
				Latency:       latency,
				Success:       false,
				ErrorCategory: ai.ErrorCategory(err),
			})
		}
		return nil, err
	}
	if reason != "" {
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:       userID,
				Operation:    aiusage.OperationJournalAnalysis,
				Provider:     s.provider,
				Model:        response.Model,
				InputTokens:  response.Usage.InputTokens,
				OutputTokens: response.Usage.OutputTokens,
				Latency:      latency,
				Success:      true,
			})
		}
		s.log.InfoContext(ctx, "journal analysis below confidence threshold",
			slog.String("job_id", jobID),
			slog.String("reason", reason),
		)
		return insufficientResult(reason), nil
	}

	analysisID, err := ids.New()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	model := response.Model
	if model == "" {
		model = s.model
	}
	promptVersion := prompts.JournalAnalysisVersion

	record := &Analysis{
		ID:                 analysisID,
		UserID:             userID,
		JournalID:          entry.ID,
		JournalVersion:     entry.Version,
		JobID:              jobID,
		Summary:            validated.Summary,
		Observation:        validated.Observation,
		Interpretation:     validated.Interpretation,
		Hypothesis:         validated.Hypothesis,
		Confidence:         validated.Confidence,
		ConfidenceLabel:    confidenceLabel(validated.Confidence),
		ReflectionQuestion: validated.ReflectionQuestion,
		SuggestedDirection: validated.SuggestedDirection,
		EvidenceNote:       validated.EvidenceNote,
		Model:              &model,
		PromptVersion:      &promptVersion,
		CreatedAt:          now,
	}

	created, err := s.repo.Create(ctx, record)
	if err != nil {
		return nil, err
	}
	if !created {
		existingVersion, err := s.repo.GetByJournalVersion(ctx, userID, entry.ID, entry.Version)
		if err != nil {
			return nil, err
		}
		if s.usage != nil {
			s.usage.Record(ctx, aiusage.Event{
				UserID:       userID,
				Operation:    aiusage.OperationJournalAnalysis,
				Provider:     s.provider,
				Model:        response.Model,
				InputTokens:  response.Usage.InputTokens,
				OutputTokens: response.Usage.OutputTokens,
				Latency:      latency,
				Success:      true,
				ReferenceID:  &existingVersion.ID,
			})
		}
		return analysisResult(existingVersion.ID), nil
	}

	if s.usage != nil {
		s.usage.Record(ctx, aiusage.Event{
			UserID:       userID,
			Operation:    aiusage.OperationJournalAnalysis,
			Provider:     s.provider,
			Model:        response.Model,
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			Latency:      latency,
			Success:      true,
			ReferenceID:  &analysisID,
		})
	}

	s.log.InfoContext(ctx, "journal analysis generated",
		slog.String("job_id", jobID),
		slog.String("analysis_id", analysisID),
		slog.String("journal_id", entry.ID),
		slog.Int("journal_version", entry.Version),
		slog.String("model", model),
		slog.Int("input_tokens", response.Usage.InputTokens),
		slog.Int("output_tokens", response.Usage.OutputTokens),
	)

	return analysisResult(analysisID), nil
}

func validateAnalysisOutput(output *analysisOutput, content string) (*analysisOutput, string, error) {
	if output == nil {
		return nil, "", fmt.Errorf("%w: empty journal analysis output", ai.ErrInvalidResponse)
	}

	output.Summary = strings.TrimSpace(output.Summary)
	output.Observation = strings.TrimSpace(output.Observation)
	output.Interpretation = strings.TrimSpace(output.Interpretation)
	output.ReflectionQuestion = strings.TrimSpace(output.ReflectionQuestion)
	output.SuggestedDirection = strings.TrimSpace(output.SuggestedDirection)
	output.EvidenceNote = strings.TrimSpace(output.EvidenceNote)

	for name, value := range map[string]string{
		"summary":        output.Summary,
		"observation":    output.Observation,
		"interpretation": output.Interpretation,
	} {
		if value == "" || utf8.RuneCountInString(value) > maxAnalysisFieldRunes {
			return nil, "", fmt.Errorf("%w: invalid analysis %s", ai.ErrInvalidResponse, name)
		}
	}
	if output.ReflectionQuestion == "" || utf8.RuneCountInString(output.ReflectionQuestion) > maxAnalysisQuestion {
		return nil, "", fmt.Errorf("%w: invalid analysis reflection question", ai.ErrInvalidResponse)
	}
	switch output.SuggestedDirection {
	case DirectionImprove, DirectionMaintain, DirectionAccept:
	default:
		return nil, "", fmt.Errorf("%w: invalid analysis direction", ai.ErrInvalidResponse)
	}
	if output.Confidence < 0 || output.Confidence > 1 {
		return nil, "", fmt.Errorf("%w: analysis confidence outside bounds", ai.ErrInvalidResponse)
	}
	if output.EvidenceNote == "" || utf8.RuneCountInString(output.EvidenceNote) > maxAnalysisEvidenceNote {
		return nil, "", fmt.Errorf("%w: invalid analysis evidence note", ai.ErrInvalidResponse)
	}
	if normalized(output.EvidenceNote) == normalized(content) {
		return nil, "", fmt.Errorf("%w: analysis evidence note copies the full entry", ai.ErrInvalidResponse)
	}

	if output.Hypothesis != nil {
		trimmed := strings.TrimSpace(*output.Hypothesis)
		if trimmed == "" {
			output.Hypothesis = nil
		} else if utf8.RuneCountInString(trimmed) > maxAnalysisHypothesis {
			return nil, "", fmt.Errorf("%w: analysis hypothesis too long", ai.ErrInvalidResponse)
		} else {
			output.Hypothesis = &trimmed
		}
	}

	if output.Confidence < minAnalysisConfidence {
		return output, "The entry does not support a confident enough analysis yet.", nil
	}
	return output, "", nil
}

func parseAnalysisResponse(content string) (*analysisOutput, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty journal analysis output", ai.ErrInvalidResponse)
	}

	var output analysisOutput
	if err := json.Unmarshal([]byte(trimmed), &output); err != nil {
		return nil, fmt.Errorf("%w: decode journal analysis output: %v", ai.ErrInvalidResponse, err)
	}
	return &output, nil
}

func analysisResult(analysisID string) *GenerationResult {
	return &GenerationResult{Outcome: OutcomeAnalysisCreated, AnalysisID: &analysisID}
}

func insufficientResult(message string) *GenerationResult {
	return &GenerationResult{Outcome: OutcomeInsufficientEvidence, Message: message}
}

func journalChangedResult(message string) *GenerationResult {
	return &GenerationResult{Outcome: OutcomeJournalChanged, Message: message}
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

func normalized(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
