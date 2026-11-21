// Package journalanalysis generates and stores per-entry journal reflections.
package journalanalysis

import "time"

// Generation outcomes.
const (
	OutcomeAnalysisCreated      = "analysis_created"
	OutcomeInsufficientEvidence = "insufficient_evidence"
	OutcomeJournalChanged       = "journal_changed"
	OutcomeBudgetExhausted      = "budget_exhausted"
)

// Confidence labels share the insight vocabulary.
const (
	ConfidenceStrong   = "strong"
	ConfidenceModerate = "moderate"
	ConfidenceLimited  = "limited"
)

// Suggested directions share the insight vocabulary.
const (
	DirectionImprove  = "improve"
	DirectionMaintain = "maintain"
	DirectionAccept   = "accept"
)

// Analysis is the persistence model for one journal analysis.
type Analysis struct {
	ID                 string    `gorm:"column:id;primaryKey;type:uuid"`
	UserID             string    `gorm:"column:user_id;type:uuid"`
	JournalID          string    `gorm:"column:journal_id;type:uuid"`
	JournalVersion     int       `gorm:"column:journal_version"`
	JobID              string    `gorm:"column:job_id;type:uuid"`
	Summary            string    `gorm:"column:summary"`
	Observation        string    `gorm:"column:observation"`
	Interpretation     string    `gorm:"column:interpretation"`
	Hypothesis         *string   `gorm:"column:hypothesis"`
	Confidence         float64   `gorm:"column:confidence"`
	ConfidenceLabel    string    `gorm:"column:confidence_label"`
	ReflectionQuestion string    `gorm:"column:reflection_question"`
	SuggestedDirection string    `gorm:"column:suggested_direction"`
	EvidenceNote       string    `gorm:"column:evidence_note"`
	Model              *string   `gorm:"column:model"`
	PromptVersion      *string   `gorm:"column:prompt_version"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (Analysis) TableName() string {
	return "journal_analyses"
}

// EvidenceView is the single entry that supports the analysis.
type EvidenceView struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	SourceDate *string `json:"source_date"`
	Note       string  `json:"note"`
}

// View is the API representation of a stored analysis.
type View struct {
	ID                 string         `json:"id"`
	JournalID          string         `json:"journal_id"`
	JournalVersion     int            `json:"journal_version"`
	CurrentVersion     int            `json:"current_journal_version"`
	Stale              bool           `json:"stale"`
	EntryDate          string         `json:"entry_date"`
	Summary            string         `json:"summary"`
	Observation        string         `json:"observation"`
	Interpretation     string         `json:"interpretation"`
	Hypothesis         *string        `json:"hypothesis"`
	Confidence         float64        `json:"confidence"`
	ConfidenceLabel    string         `json:"confidence_label"`
	ReflectionQuestion string         `json:"reflection_question"`
	SuggestedDirection string         `json:"suggested_direction"`
	Evidence           []EvidenceView `json:"evidence"`
	Model              *string        `json:"model"`
	PromptVersion      *string        `json:"prompt_version"`
	CreatedAt          time.Time      `json:"created_at"`
}

// JobPayload is stored in scheduled_jobs.payload.
type JobPayload struct {
	JournalID      string `json:"journal_id"`
	JournalVersion int    `json:"journal_version"`
}

// GenerationResult is stored in scheduled_jobs.result.
type GenerationResult struct {
	Outcome    string  `json:"outcome"`
	AnalysisID *string `json:"analysis_id,omitempty"`
	Message    string  `json:"message,omitempty"`
}

// AcceptedView is the API representation of an accepted analysis request.
type AcceptedView struct {
	JobID             string `json:"job_id"`
	Status            string `json:"status"`
	StatusURL         string `json:"status_url"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

// JobView is the API representation of an analysis job.
type JobView struct {
	JobID             string    `json:"job_id"`
	Status            string    `json:"status"`
	Outcome           *string   `json:"outcome"`
	AnalysisID        *string   `json:"analysis_id"`
	Message           *string   `json:"message"`
	Error             *JobError `json:"error"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// JobError describes a failed analysis job safely.
type JobError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Limits holds per-entry analysis rate limits.
type Limits struct {
	PerHour int
	PerDay  int
}

func newView(row *latestRow) *View {
	entryDate := row.EntryDate.Format("2006-01-02")
	evidence := []EvidenceView{{
		SourceType: "journal",
		SourceID:   row.JournalID,
		SourceDate: &entryDate,
		Note:       row.EvidenceNote,
	}}

	return &View{
		ID:                 row.ID,
		JournalID:          row.JournalID,
		JournalVersion:     row.JournalVersion,
		CurrentVersion:     row.CurrentVersion,
		Stale:              row.JournalVersion != row.CurrentVersion,
		EntryDate:          entryDate,
		Summary:            row.Summary,
		Observation:        row.Observation,
		Interpretation:     row.Interpretation,
		Hypothesis:         row.Hypothesis,
		Confidence:         row.Confidence,
		ConfidenceLabel:    row.ConfidenceLabel,
		ReflectionQuestion: row.ReflectionQuestion,
		SuggestedDirection: row.SuggestedDirection,
		Evidence:           evidence,
		Model:              row.Model,
		PromptVersion:      row.PromptVersion,
		CreatedAt:          row.CreatedAt,
	}
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
