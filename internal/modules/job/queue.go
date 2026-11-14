package job

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

const defaultMaxAttempts = 3
const staleLockAge = 10 * time.Minute

// Queue persists and claims scheduled jobs.
type Queue struct {
	db *gorm.DB
}

// NewQueue creates the job queue.
func NewQueue(db *gorm.DB) *Queue {
	return &Queue{db: db}
}

// Transaction runs fn in a database transaction.
func (q *Queue) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return q.db.WithContext(ctx).Transaction(fn)
}

// EnqueueJournalEmbedding adds an idempotent embedding job inside a
// transaction. Re-enqueuing the same journal content is a no-op.
func (q *Queue) EnqueueJournalEmbedding(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash string, runAt time.Time) error {
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalEmbedding, runAt)
}

// EnqueueJournalMemoryExtraction adds an idempotent memory extraction job
// inside a transaction.
func (q *Queue) EnqueueJournalMemoryExtraction(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash string, runAt time.Time) error {
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalMemoryExtraction, runAt)
}

func (q *Queue) enqueueJournalJob(
	ctx context.Context,
	tx *gorm.DB,
	userID, journalID, contentHash, jobType string,
	runAt time.Time,
) error {
	payload, err := json.Marshal(map[string]string{"journal_id": journalID})
	if err != nil {
		return fmt.Errorf("encode journal job payload: %w", err)
	}

	id, err := ids.New()
	if err != nil {
		return err
	}

	key := uuid.NewSHA1(uuid.NameSpaceOID, []byte(jobType+":"+journalID+":"+contentHash)).String()
	now := time.Now().UTC()

	return tx.Exec(
		`INSERT INTO scheduled_jobs
		 (id, user_id, job_type, payload, status, attempts, max_attempts, run_at, idempotency_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?::jsonb, 'pending', 0, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		id, userID, jobType, string(payload), defaultMaxAttempts, runAt, key, now, now,
	).Error
}

// Enqueue adds a generic job outside a transaction.
func (q *Queue) Enqueue(ctx context.Context, job *Job) error {
	if job.ID == "" {
		id, err := ids.New()
		if err != nil {
			return err
		}
		job.ID = id
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultMaxAttempts
	}
	if job.RunAt.IsZero() {
		job.RunAt = time.Now().UTC()
	}
	if job.Status == "" {
		job.Status = StatusPending
	}

	payload := "{}"
	if len(job.Payload) > 0 {
		payload = string(job.Payload)
	}

	return q.db.WithContext(ctx).Exec(
		`INSERT INTO scheduled_jobs
		 (id, user_id, job_type, payload, status, attempts, max_attempts, run_at, idempotency_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?::jsonb, ?, 0, ?, ?, ?, ?, ?)`,
		job.ID, job.UserID, job.JobType, payload, job.Status, job.MaxAttempts,
		job.RunAt, job.IdempotencyKey, time.Now().UTC(), time.Now().UTC(),
	).Error
}

// Claim locks one eligible job for the worker. It also returns stale
// processing jobs to the queue.
func (q *Queue) Claim(ctx context.Context, workerID string) (*Job, error) {
	if err := q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs
		 SET status = 'retry', locked_at = NULL, locked_by = NULL, updated_at = now()
		 WHERE status = 'processing' AND locked_at < ?`,
		time.Now().UTC().Add(-staleLockAge),
	).Error; err != nil {
		return nil, fmt.Errorf("recover stale jobs: %w", err)
	}

	var id string
	row := q.db.WithContext(ctx).Raw(
		`WITH candidate AS (
		   SELECT id
		   FROM scheduled_jobs
		   WHERE status IN ('pending', 'retry')
		     AND run_at <= now()
		     AND attempts < max_attempts
		   ORDER BY run_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 )
		 UPDATE scheduled_jobs j
		 SET status = 'processing',
		     locked_at = now(),
		     locked_by = ?,
		     attempts = attempts + 1,
		     updated_at = now()
		 FROM candidate
		 WHERE j.id = candidate.id
		 RETURNING j.id`,
		workerID,
	).Row()

	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim job: %w", err)
	}

	var claimed Job
	if err := q.db.WithContext(ctx).Where("id = ?", id).Take(&claimed).Error; err != nil {
		return nil, fmt.Errorf("load claimed job: %w", err)
	}

	return &claimed, nil
}

// Complete marks a job completed.
func (q *Queue) Complete(ctx context.Context, jobID string) error {
	return q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs
		 SET status = 'completed', locked_at = NULL, locked_by = NULL, last_error = NULL, updated_at = now()
		 WHERE id = ?`,
		jobID,
	).Error
}

// Retry schedules another attempt.
func (q *Queue) Retry(ctx context.Context, jobID, message string, runAt time.Time) error {
	return q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs
		 SET status = 'retry', locked_at = NULL, locked_by = NULL, last_error = ?, run_at = ?, updated_at = now()
		 WHERE id = ?`,
		message, runAt, jobID,
	).Error
}

// Fail marks a job failed and records the safe error category.
func (q *Queue) Fail(ctx context.Context, jobID, message string) error {
	return q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs
		 SET status = 'failed', locked_at = NULL, locked_by = NULL, last_error = ?, updated_at = now()
		 WHERE id = ?`,
		message, jobID,
	).Error
}
