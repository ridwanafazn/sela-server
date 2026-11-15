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
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalEmbedding, "", runAt)
}

// EnqueueJournalEmbeddingForModel adds an embedding job keyed by model so a
// model migration can re-embed journals that already have vectors.
func (q *Queue) EnqueueJournalEmbeddingForModel(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash, model string, runAt time.Time) error {
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalEmbedding, "model:"+model, runAt)
}

// EnqueueJournalMemoryExtraction adds an idempotent memory extraction job
// inside a transaction.
func (q *Queue) EnqueueJournalMemoryExtraction(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash string, runAt time.Time) error {
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalMemoryExtraction, "", runAt)
}

// EnqueueJournalMemoryExtractionForVersion adds an extraction job keyed by
// prompt version so a prompt migration can re-extract journals.
func (q *Queue) EnqueueJournalMemoryExtractionForVersion(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash, promptVersion string, runAt time.Time) error {
	return q.enqueueJournalJob(ctx, tx, userID, journalID, contentHash, TypeJournalMemoryExtraction, "prompt:"+promptVersion, runAt)
}

// EnqueueMessageMemoryExtraction adds an idempotent extraction job for a user
// message inside a transaction.
func (q *Queue) EnqueueMessageMemoryExtraction(ctx context.Context, tx *gorm.DB, userID, messageID, contentHash string, runAt time.Time) error {
	return q.enqueueJob(
		ctx, tx, userID, TypeMessageMemoryExtraction,
		map[string]string{"message_id": messageID},
		[]string{TypeMessageMemoryExtraction, messageID, contentHash},
		runAt,
	)
}

func (q *Queue) enqueueJournalJob(
	ctx context.Context,
	tx *gorm.DB,
	userID, journalID, contentHash, jobType, variant string,
	runAt time.Time,
) error {
	parts := []string{jobType, journalID, contentHash}
	if variant != "" {
		parts = append(parts, variant)
	}
	return q.enqueueJob(
		ctx, tx, userID, jobType,
		map[string]string{"journal_id": journalID},
		parts,
		runAt,
	)
}

func (q *Queue) enqueueJob(
	ctx context.Context,
	tx *gorm.DB,
	userID, jobType string,
	payload map[string]string,
	keyParts []string,
	runAt time.Time,
) error {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode job payload: %w", err)
	}

	id, err := ids.New()
	if err != nil {
		return err
	}

	key := uuid.NewSHA1(uuid.NameSpaceOID, []byte(joinKeyParts(keyParts))).String()
	now := time.Now().UTC()

	return tx.Exec(
		`INSERT INTO scheduled_jobs
		 (id, user_id, job_type, payload, status, attempts, max_attempts, run_at, idempotency_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?::jsonb, 'pending', 0, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		id, userID, jobType, string(encodedPayload), defaultMaxAttempts, runAt, key, now, now,
	).Error
}

func joinKeyParts(parts []string) string {
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += ":"
		}
		result += part
	}
	return result
}

// EnqueueIdempotent inserts a job or returns the existing job for the same
// user and idempotency key.
func (q *Queue) EnqueueIdempotent(ctx context.Context, job *Job) (string, bool, error) {
	if job.UserID == nil || *job.UserID == "" {
		return "", false, fmt.Errorf("job user_id is required")
	}
	if job.IdempotencyKey == nil || *job.IdempotencyKey == "" {
		return "", false, fmt.Errorf("job idempotency key is required")
	}
	if job.ID == "" {
		id, err := ids.New()
		if err != nil {
			return "", false, err
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

	result := q.db.WithContext(ctx).Exec(
		`INSERT INTO scheduled_jobs
		 (id, user_id, job_type, payload, status, attempts, max_attempts, run_at, idempotency_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?::jsonb, ?, 0, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		job.ID, *job.UserID, job.JobType, payload, job.Status, job.MaxAttempts,
		job.RunAt, *job.IdempotencyKey, time.Now().UTC(), time.Now().UTC(),
	)
	if result.Error != nil {
		return "", false, result.Error
	}
	if result.RowsAffected == 1 {
		return job.ID, true, nil
	}

	existing, err := q.FindByUserAndKey(ctx, *job.UserID, *job.IdempotencyKey)
	if err != nil {
		return "", false, err
	}
	return existing.ID, false, nil
}

// Get returns one user-owned job.
func (q *Queue) Get(ctx context.Context, userID, jobID string) (*Job, error) {
	var claimed Job
	if err := q.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", jobID, userID).
		Take(&claimed).Error; err != nil {
		return nil, err
	}
	return &claimed, nil
}

// FindByUserAndKey returns a job by its user-scoped idempotency key.
func (q *Queue) FindByUserAndKey(ctx context.Context, userID, key string) (*Job, error) {
	var claimed Job
	if err := q.db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ?", userID, key).
		Take(&claimed).Error; err != nil {
		return nil, err
	}
	return &claimed, nil
}

// SetResult stores a structured job outcome.
func (q *Queue) SetResult(ctx context.Context, jobID string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode job result: %w", err)
	}
	return q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs SET result = ?::jsonb, updated_at = now() WHERE id = ?`,
		string(encoded), jobID,
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
		 SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'retry' END,
		     locked_at = NULL,
		     locked_by = NULL,
		     last_error = CASE
		       WHEN attempts >= max_attempts THEN 'job lock expired after final attempt'
		       ELSE last_error
		     END,
		     result = NULL,
		     updated_at = now()
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
		 SET status = 'retry', locked_at = NULL, locked_by = NULL, last_error = ?, run_at = ?, result = NULL, updated_at = now()
		 WHERE id = ?`,
		message, runAt, jobID,
	).Error
}

// Fail marks a job failed and records the safe error category.
func (q *Queue) Fail(ctx context.Context, jobID, message string) error {
	return q.db.WithContext(ctx).Exec(
		`UPDATE scheduled_jobs
		 SET status = 'failed', locked_at = NULL, locked_by = NULL, last_error = ?, result = NULL, updated_at = now()
		 WHERE id = ?`,
		message, jobID,
	).Error
}
