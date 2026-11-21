// Package memory stores provenance-aware personal memories, extracts
// candidates from journals, and exposes review endpoints.
package memory

import "time"

// Memory statuses.
const (
	StatusCandidate  = "candidate"
	StatusActive     = "active"
	StatusRejected   = "rejected"
	StatusSuperseded = "superseded"
	StatusDeleted    = "deleted"
)

// Memory sensitivity.
const (
	SensitivityNormal    = "normal"
	SensitivitySensitive = "sensitive"
)

// ConsentScopeSensitiveMemory is the explicit consent required to store and
// retrieve sensitive memories.
const ConsentScopeSensitiveMemory = "sensitive_memory"

// Memory is the persistence model for a personal memory.
type Memory struct {
	ID                 string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID             string     `gorm:"column:user_id;type:uuid"`
	Content            string     `gorm:"column:content"`
	Category           *string    `gorm:"column:category"`
	Sensitivity        string     `gorm:"column:sensitivity"`
	Status             string     `gorm:"column:status"`
	Confidence         *float64   `gorm:"column:confidence"`
	SourceCount        int        `gorm:"column:source_count"`
	SupersedesMemoryID *string    `gorm:"column:supersedes_memory_id;type:uuid"`
	LastConfirmedAt    *time.Time `gorm:"column:last_confirmed_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	DeletedAt          *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (Memory) TableName() string {
	return "personal_memories"
}

// MemorySource links a memory to a supporting journal or message.
type MemorySource struct {
	ID            string    `gorm:"column:id;primaryKey;type:uuid"`
	MemoryID      string    `gorm:"column:memory_id;type:uuid"`
	JournalID     *string   `gorm:"column:journal_id;type:uuid"`
	MessageID     *string   `gorm:"column:message_id;type:uuid"`
	SourceExcerpt *string   `gorm:"column:source_excerpt"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (MemorySource) TableName() string {
	return "memory_sources"
}

// JournalSource is the eligible journal text used for extraction.
type JournalSource struct {
	ID        string     `gorm:"column:id"`
	UserID    string     `gorm:"column:user_id"`
	Title     *string    `gorm:"column:title"`
	Content   string     `gorm:"column:content"`
	Status    string     `gorm:"column:status"`
	EntryDate time.Time  `gorm:"column:entry_date"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

// MessageSource is the eligible user message text used for extraction.
type MessageSource struct {
	ID                    string     `gorm:"column:id"`
	UserID                string     `gorm:"column:user_id"`
	ConversationID        string     `gorm:"column:conversation_id"`
	Role                  string     `gorm:"column:role"`
	Content               string     `gorm:"column:content"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	ConversationDeletedAt *time.Time `gorm:"column:conversation_deleted_at"`
}

// SourceRef identifies one provenance record.
type SourceRef struct {
	JournalID *string
	MessageID *string
}

// View is the API representation of a memory.
type View struct {
	ID                 string     `json:"id"`
	Content            string     `json:"content"`
	Category           *string    `json:"category"`
	Sensitivity        string     `json:"sensitivity"`
	Status             string     `json:"status"`
	Confidence         float64    `json:"confidence"`
	SourceCount        int        `json:"source_count"`
	SupersedesMemoryID *string    `json:"supersedes_memory_id"`
	LastConfirmedAt    *time.Time `json:"last_confirmed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// SourceView is the API representation of a memory source.
type SourceView struct {
	ID            string     `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	SourceDate    *time.Time `json:"source_date"`
	Excerpt       *string    `json:"excerpt"`
	SourceDeleted bool       `json:"source_deleted"`
}

// Detail is a memory with its provenance.
type Detail struct {
	View
	Sources       []SourceView `json:"sources"`
	ConflictsWith []string     `json:"conflicts_with"`
}

// Page is a page of memory views.
type Page struct {
	Items      []View
	NextCursor *string
	HasMore    bool
}

// ListFilter is a validated memory list query.
type ListFilter struct {
	Status   string
	Cursor   string
	Category string
	Limit    int
}

// UpdateInput is a validated partial update.
type UpdateInput struct {
	Content     *string
	Category    *string
	CategorySet bool
}

// FeedbackInput is a validated memory review action.
type FeedbackInput struct {
	Action           string
	CorrectedContent *string
}

// RetrievedMemory is a memory ranked for Companion context.
type RetrievedMemory struct {
	ID          string
	Content     string
	Category    *string
	Sensitivity string
	Status      string
	Confidence  float64
	UpdatedAt   time.Time
}

// Consent records explicit, revocable user permission for a scoped use of
// personal data.
type Consent struct {
	ID        string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	Scope     string     `gorm:"column:scope"`
	Granted   bool       `gorm:"column:granted"`
	GrantedAt *time.Time `gorm:"column:granted_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (Consent) TableName() string {
	return "memory_consents"
}

// ConsentView is the API representation of a memory consent.
type ConsentView struct {
	Scope     string     `json:"scope"`
	Granted   bool       `json:"granted"`
	GrantedAt *time.Time `json:"granted_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func newView(memory *Memory) View {
	confidence := 0.0
	if memory.Confidence != nil {
		confidence = *memory.Confidence
	}
	return View{
		ID:                 memory.ID,
		Content:            memory.Content,
		Category:           memory.Category,
		Sensitivity:        memory.Sensitivity,
		Status:             memory.Status,
		Confidence:         confidence,
		SourceCount:        memory.SourceCount,
		SupersedesMemoryID: memory.SupersedesMemoryID,
		LastConfirmedAt:    memory.LastConfirmedAt,
		CreatedAt:          memory.CreatedAt,
		UpdatedAt:          memory.UpdatedAt,
	}
}
