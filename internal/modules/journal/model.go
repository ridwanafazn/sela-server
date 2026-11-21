package journal

import (
	"time"

	"github.com/ridwanafazn/sela-server/internal/shared/optional"
)

// Journal statuses.
const (
	StatusDraft  = "draft"
	StatusActive = "active"
)

// Journal is the persistence model for a journal entry.
type Journal struct {
	ID        string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	Title     *string    `gorm:"column:title"`
	Content   string     `gorm:"column:content"`
	EntryDate time.Time  `gorm:"column:entry_date;type:date"`
	Status    string     `gorm:"column:status"`
	Version   int        `gorm:"column:version"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

// TableName pins the table name.
func (Journal) TableName() string {
	return "journals"
}

// BackfillTarget is an active journal queued for derived-work regeneration.
type BackfillTarget struct {
	ID      string `gorm:"column:id"`
	UserID  string `gorm:"column:user_id"`
	Content string `gorm:"column:content"`
}

// View is the API representation of a journal.
type View struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title"`
	Content   string    `json:"content"`
	EntryDate string    `json:"entry_date"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateInput is a validated create request.
type CreateInput struct {
	Title     *string
	Content   string
	EntryDate *string
	Status    *string
}

// UpdateInput is a validated partial update request.
type UpdateInput struct {
	Title     optional.String
	Content   optional.String
	EntryDate optional.String
	Status    optional.String
}

// IsEmpty reports whether the update changes nothing.
func (i UpdateInput) IsEmpty() bool {
	return !i.Title.Set && !i.Content.Set && !i.EntryDate.Set && !i.Status.Set
}

// ListFilter is a validated journal list query.
type ListFilter struct {
	Cursor string
	Limit  int
	Query  string
	Status string
	From   *time.Time
	To     *time.Time
}

// Page is a page of journal views.
type Page struct {
	Items      []View
	NextCursor *string
	HasMore    bool
}

// ChangeFilter is a validated journal-changes query.
type ChangeFilter struct {
	Cursor string
	Limit  int
}

// JournalChange is one current journal record for offline synchronization.
// Deleted records are tombstones with a nil journal.
type JournalChange struct {
	ID        string     `json:"id"`
	Version   int        `json:"version"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
	Journal   *View      `json:"journal"`
}

// ChangePage is a page of journal changes and the server sync time.
type ChangePage struct {
	Items      []JournalChange
	NextCursor *string
	HasMore    bool
	ServerTime time.Time
}

// IdempotencyRecord is a stored replayable response.
type IdempotencyRecord struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	UserID         string    `gorm:"column:user_id;type:uuid"`
	Key            string    `gorm:"column:key"`
	Endpoint       string    `gorm:"column:endpoint"`
	RequestHash    string    `gorm:"column:request_hash"`
	ResponseStatus *int      `gorm:"column:response_status"`
	ResponseBody   []byte    `gorm:"column:response_body"`
	JobID          *string   `gorm:"column:job_id;type:uuid"`
	ExpiresAt      time.Time `gorm:"column:expires_at"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (IdempotencyRecord) TableName() string {
	return "idempotency_keys"
}

func newView(journal *Journal) View {
	return View{
		ID:        journal.ID,
		Title:     journal.Title,
		Content:   journal.Content,
		EntryDate: journal.EntryDate.Format("2006-01-02"),
		Status:    journal.Status,
		Version:   journal.Version,
		CreatedAt: journal.CreatedAt,
		UpdatedAt: journal.UpdatedAt,
	}
}
