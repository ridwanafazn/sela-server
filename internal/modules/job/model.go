// Package job implements durable, database-backed background jobs.
package job

import (
	"context"
	"time"
)

// Job statuses.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusRetry      = "retry"
	StatusFailed     = "failed"
)

// Job types.
const (
	TypeJournalEmbedding        = "journal.embedding"
	TypeJournalMemoryExtraction = "journal.memory_extraction"
	TypeMessageMemoryExtraction = "message.memory_extraction"
	TypeInsightGenerate         = "insight.generate"
	TypeAccountDelete           = "account.delete"
)

// Job is a durable scheduled task.
type Job struct {
	ID             string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID         *string    `gorm:"column:user_id;type:uuid"`
	JobType        string     `gorm:"column:job_type"`
	Payload        []byte     `gorm:"column:payload"`
	Status         string     `gorm:"column:status"`
	Attempts       int        `gorm:"column:attempts"`
	MaxAttempts    int        `gorm:"column:max_attempts"`
	RunAt          time.Time  `gorm:"column:run_at"`
	LockedAt       *time.Time `gorm:"column:locked_at"`
	LockedBy       *string    `gorm:"column:locked_by"`
	LastError      *string    `gorm:"column:last_error"`
	Result         []byte     `gorm:"column:result"`
	IdempotencyKey *string    `gorm:"column:idempotency_key;type:uuid"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (Job) TableName() string {
	return "scheduled_jobs"
}

// Handler processes one job.
type Handler interface {
	Handle(ctx context.Context, job *Job) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, job *Job) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, job *Job) error {
	return f(ctx, job)
}
