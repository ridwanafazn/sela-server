// Package insight generates asynchronous, evidence-backed reflections and
// manages insight feedback.
package insight

import "time"

// Generation types.
const (
	GenerationOnDemand = "on_demand"
	GenerationWeekly   = "weekly"
	GenerationMonthly  = "monthly"
)

// Confidence labels.
const (
	ConfidenceStrong   = "strong"
	ConfidenceModerate = "moderate"
	ConfidenceLimited  = "limited"
)

// Suggested directions.
const (
	DirectionImprove  = "improve"
	DirectionMaintain = "maintain"
	DirectionAccept   = "accept"
)

// Feedback statuses.
const (
	FeedbackNone      = "none"
	FeedbackConfirmed = "confirmed"
	FeedbackCorrected = "corrected"
	FeedbackRejected  = "rejected"
)

// Insight is the persistence model for a completed insight.
type Insight struct {
	ID                 string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID             string     `gorm:"column:user_id;type:uuid"`
	JobID              *string    `gorm:"column:job_id;type:uuid"`
	Title              string     `gorm:"column:title"`
	Summary            string     `gorm:"column:summary"`
	Observation        string     `gorm:"column:observation"`
	Interpretation     string     `gorm:"column:interpretation"`
	Hypothesis         *string    `gorm:"column:hypothesis"`
	Confidence         float64    `gorm:"column:confidence"`
	ConfidenceLabel    string     `gorm:"column:confidence_label"`
	ReflectionQuestion string     `gorm:"column:reflection_question"`
	SuggestedDirection string     `gorm:"column:suggested_direction"`
	PeriodStart        *time.Time `gorm:"column:period_start;type:date"`
	PeriodEnd          *time.Time `gorm:"column:period_end;type:date"`
	GenerationType     string     `gorm:"column:generation_type"`
	Model              *string    `gorm:"column:model"`
	PromptVersion      *string    `gorm:"column:prompt_version"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	DeletedAt          *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (Insight) TableName() string {
	return "insights"
}

// Evidence links an insight to one supporting record.
type Evidence struct {
	ID              string     `gorm:"column:id;primaryKey;type:uuid"`
	InsightID       string     `gorm:"column:insight_id;type:uuid"`
	JournalID       *string    `gorm:"column:journal_id;type:uuid"`
	MessageID       *string    `gorm:"column:message_id;type:uuid"`
	MemoryID        *string    `gorm:"column:memory_id;type:uuid"`
	Note            string     `gorm:"column:note"`
	SourceDeletedAt *time.Time `gorm:"column:source_deleted_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
}

// TableName pins the table name.
func (Evidence) TableName() string {
	return "insight_evidence"
}

// Feedback is an append-only user assessment.
type Feedback struct {
	ID        string    `gorm:"column:id;primaryKey;type:uuid"`
	InsightID string    `gorm:"column:insight_id;type:uuid"`
	UserID    string    `gorm:"column:user_id;type:uuid"`
	Action    string    `gorm:"column:action"`
	Note      *string   `gorm:"column:note"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (Feedback) TableName() string {
	return "insight_feedback"
}

// SourceRecord is one eligible record included in an insight prompt.
type SourceRecord struct {
	Type     string
	ID       string
	Date     time.Time
	Content  string
	Title    *string
	Category *string
}

// View is the API representation of a completed insight.
type View struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Summary            string         `json:"summary"`
	Observation        string         `json:"observation"`
	Interpretation     string         `json:"interpretation"`
	Hypothesis         *string        `json:"hypothesis"`
	Confidence         float64        `json:"confidence"`
	ConfidenceLabel    string         `json:"confidence_label"`
	ReflectionQuestion string         `json:"reflection_question"`
	SuggestedDirection string         `json:"suggested_direction"`
	PeriodStart        *string        `json:"period_start"`
	PeriodEnd          *string        `json:"period_end"`
	GenerationType     string         `json:"generation_type"`
	Evidence           []EvidenceView `json:"evidence"`
	Feedback           FeedbackView   `json:"feedback"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// ListItem is the API representation of an insight summary.
type ListItem struct {
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Summary            string       `json:"summary"`
	Confidence         float64      `json:"confidence"`
	ConfidenceLabel    string       `json:"confidence_label"`
	SuggestedDirection string       `json:"suggested_direction"`
	PeriodStart        *string      `json:"period_start"`
	PeriodEnd          *string      `json:"period_end"`
	GenerationType     string       `json:"generation_type"`
	EvidenceCount      int          `json:"evidence_count"`
	Feedback           FeedbackView `json:"feedback"`
	CreatedAt          time.Time    `json:"created_at"`
}

// EvidenceView is the API representation of insight evidence.
type EvidenceView struct {
	ID            string     `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	Note          string     `json:"note"`
	SourceDate    *time.Time `json:"source_date"`
	SourceDeleted bool       `json:"source_deleted"`
}

// FeedbackView is the latest insight feedback.
type FeedbackView struct {
	Status    string     `json:"status"`
	Note      *string    `json:"note"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// Detail is an insight with its evidence and feedback.
type Detail struct {
	View
}

// Page is a page of insight summaries.
type Page struct {
	Items      []ListItem
	NextCursor *string
	HasMore    bool
}

// GenerateInput is a validated insight generation request.
type GenerateInput struct {
	Period    string
	StartDate *string
	EndDate   *string
	Focus     string
}

// ListFilter is a validated insight list query.
type ListFilter struct {
	Period string
	Cursor string
	Limit  int
}

// FeedbackInput is a validated insight feedback request.
type FeedbackInput struct {
	Action string
	Note   *string
}

// Limits holds insight generation rate limits.
type Limits struct {
	PerHour int
	PerDay  int
}

// JobPayload is stored in scheduled_jobs.payload.
type JobPayload struct {
	Period         string `json:"period"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	Focus          string `json:"focus"`
	GenerationType string `json:"generation_type"`
	RequestHash    string `json:"request_hash"`
	ScheduleID     string `json:"schedule_id,omitempty"`
}

// GenerationResult is stored in scheduled_jobs.result.
type GenerationResult struct {
	Outcome   string  `json:"outcome"`
	InsightID *string `json:"insight_id,omitempty"`
	Message   string  `json:"message,omitempty"`
}

// JobView is the API representation of an insight job.
type JobView struct {
	JobID             string    `json:"job_id"`
	Status            string    `json:"status"`
	Outcome           *string   `json:"outcome"`
	InsightID         *string   `json:"insight_id"`
	Message           *string   `json:"message"`
	Error             *JobError `json:"error"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// JobError describes a failed insight job safely.
type JobError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// AcceptedView is the API representation of an accepted generation request.
type AcceptedView struct {
	JobID             string `json:"job_id"`
	Status            string `json:"status"`
	StatusURL         string `json:"status_url"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

func newView(insight *Insight, evidence []EvidenceView, feedback FeedbackView) View {
	return View{
		ID:                 insight.ID,
		Title:              insight.Title,
		Summary:            insight.Summary,
		Observation:        insight.Observation,
		Interpretation:     insight.Interpretation,
		Hypothesis:         insight.Hypothesis,
		Confidence:         insight.Confidence,
		ConfidenceLabel:    insight.ConfidenceLabel,
		ReflectionQuestion: insight.ReflectionQuestion,
		SuggestedDirection: insight.SuggestedDirection,
		PeriodStart:        dateString(insight.PeriodStart),
		PeriodEnd:          dateString(insight.PeriodEnd),
		GenerationType:     insight.GenerationType,
		Evidence:           evidence,
		Feedback:           feedback,
		CreatedAt:          insight.CreatedAt,
		UpdatedAt:          insight.UpdatedAt,
	}
}

func dateString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}
