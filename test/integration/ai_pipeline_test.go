//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
)

func processAllJobs(t *testing.T, worker *job.Worker) {
	t.Helper()

	for {
		processed, err := worker.ProcessOnce(context.Background())
		if err != nil {
			t.Fatalf("process job: %v", err)
		}
		if !processed {
			return
		}
	}
}

func TestEmbeddingPipelineAndHybridRetrieval(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	sleepJournal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Late night",
		"content":    "I could not sleep well and kept thinking about work.",
		"entry_date": "2026-11-10",
	})
	walkJournal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "A walk",
		"content":    "A calm walk by the river helped me reset.",
		"entry_date": "2026-11-09",
	})
	draft := mustCreateDraft(t, router, user.AccessToken, "sleep draft that must stay out of retrieval")

	var pending int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_jobs WHERE user_id = $1 AND job_type = $2 AND status = 'pending'`,
		user.User.ID, job.TypeJournalEmbedding,
	).Scan(&pending); err != nil {
		t.Fatalf("count embedding jobs: %v", err)
	}
	if pending != 2 {
		t.Fatalf("pending embedding jobs = %d, want 2 (active journals only)", pending)
	}
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_jobs WHERE user_id = $1 AND job_type = $2 AND status = 'pending'`,
		user.User.ID, job.TypeJournalMemoryExtraction,
	).Scan(&pending); err != nil {
		t.Fatalf("count extraction jobs: %v", err)
	}
	if pending != 2 {
		t.Fatalf("pending memory extraction jobs = %d, want 2 (active journals only)", pending)
	}

	repo := journal.NewRepository(db.Gorm)
	embeddings := ai.NewFakeEmbedding(768)
	embeddingService := journal.NewEmbeddingService(repo, embeddings, "fake-embedding-768", 768, testLogger())

	queue := job.NewQueue(db.Gorm)
	worker := job.NewWorker(queue, map[string]job.Handler{
		job.TypeJournalEmbedding: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				JournalID string `json:"journal_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return err
			}
			return embeddingService.ProcessJournal(ctx, payload.JournalID)
		}),
		job.TypeJournalMemoryExtraction: job.HandlerFunc(func(context.Context, *job.Job) error {
			return nil
		}),
		"test.retry": job.HandlerFunc(func(context.Context, *job.Job) error {
			return ai.ErrRateLimited
		}),
		"test.fail": job.HandlerFunc(func(context.Context, *job.Job) error {
			return ai.ErrInvalidRequest
		}),
	}, "test-worker", time.Millisecond, testLogger())

	processAllJobs(t, worker)

	for _, journalID := range []string{sleepJournal.ID, walkJournal.ID} {
		var chunkCount, dimensions int
		if err := db.SQL.QueryRowContext(ctx,
			`SELECT count(*), min(vector_dims(embedding)) FROM journal_embeddings WHERE journal_id = $1`,
			journalID,
		).Scan(&chunkCount, &dimensions); err != nil {
			t.Fatalf("load embeddings: %v", err)
		}
		if chunkCount < 1 {
			t.Errorf("journal %s has no embeddings", journalID)
		}
		if dimensions != 768 {
			t.Errorf("journal %s dimensions = %d, want 768", journalID, dimensions)
		}
	}

	var draftEmbeddings int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM journal_embeddings WHERE journal_id = $1`, draft.ID,
	).Scan(&draftEmbeddings); err != nil {
		t.Fatalf("count draft embeddings: %v", err)
	}
	if draftEmbeddings != 0 {
		t.Errorf("draft has %d embeddings, want 0", draftEmbeddings)
	}

	retrieval := journal.NewRetrievalService(repo, embeddings, "fake-embedding-768", 768, testLogger())

	// Exact content matches the vector and the full-text index.
	results, err := retrieval.Retrieve(ctx, user.User.ID, sleepJournal.Content, 8)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) == 0 || results[0].JournalID != sleepJournal.ID {
		t.Fatalf("top result = %+v, want journal %s", results, sleepJournal.ID)
	}
	if results[0].VectorScore <= 0 {
		t.Errorf("vector score = %f, want > 0", results[0].VectorScore)
	}
	for _, result := range results {
		if result.JournalID == draft.ID {
			t.Error("draft appeared in retrieval results")
		}
	}

	// Text-only query still finds the sleep journal and excludes the draft.
	textResults, err := retrieval.Retrieve(ctx, user.User.ID, "sleep", 8)
	if err != nil {
		t.Fatalf("text retrieve: %v", err)
	}
	foundSleep := false
	for _, result := range textResults {
		if result.JournalID == sleepJournal.ID {
			foundSleep = true
		}
		if result.JournalID == draft.ID {
			t.Error("draft appeared in text retrieval results")
		}
	}
	if !foundSleep {
		t.Error("sleep journal was not retrieved by full-text search")
	}

	naturalResults, err := retrieval.Retrieve(ctx, user.User.ID, "What have I written about sleep?", 8)
	if err != nil {
		t.Fatalf("natural language retrieve: %v", err)
	}
	foundNatural := false
	for _, result := range naturalResults {
		if result.JournalID == sleepJournal.ID {
			foundNatural = true
		}
	}
	if !foundNatural {
		t.Error("natural-language query did not retrieve the sleep journal")
	}

	// A different user must not see these journals.
	otherUser := newTestUser(t, router)
	otherResults, err := retrieval.Retrieve(ctx, otherUser.User.ID, sleepJournal.Content, 8)
	if err != nil {
		t.Fatalf("other user retrieve: %v", err)
	}
	if len(otherResults) != 0 {
		t.Errorf("other user retrieved %d journals, want 0", len(otherResults))
	}

	// Updating content enqueues a new embedding job and replaces vectors.
	var oldHash string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT content_hash FROM journal_embeddings WHERE journal_id = $1 ORDER BY chunk_index LIMIT 1`,
		sleepJournal.ID,
	).Scan(&oldHash); err != nil {
		t.Fatalf("load old hash: %v", err)
	}

	updated := do(t, router, http.MethodPatch, journalAPIPath(sleepJournal.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body: map[string]any{
			"content": "Updated reflection about rest and recovery.",
		},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update journal = %d: %s", updated.Status, updated.Body)
	}

	processAllJobs(t, worker)

	var newHash string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT content_hash FROM journal_embeddings WHERE journal_id = $1 ORDER BY chunk_index LIMIT 1`,
		sleepJournal.ID,
	).Scan(&newHash); err != nil {
		t.Fatalf("load new hash: %v", err)
	}
	if newHash == oldHash {
		t.Error("content update did not replace the embedding hash")
	}

	// Deleting a journal removes its derived vectors.
	deleted := do(t, router, http.MethodDelete, journalAPIPath(sleepJournal.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "2"},
	})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete journal = %d: %s", deleted.Status, deleted.Body)
	}
	var afterDelete int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM journal_embeddings WHERE journal_id = $1`, sleepJournal.ID,
	).Scan(&afterDelete); err != nil {
		t.Fatalf("count deleted embeddings: %v", err)
	}
	if afterDelete != 0 {
		t.Errorf("embeddings after delete = %d, want 0", afterDelete)
	}
}

func TestWorkerRetriesAndFailsPermanently(t *testing.T) {
	_, db := newAPI(t, testConfig())
	ctx := context.Background()

	queue := job.NewQueue(db.Gorm)
	if err := queue.Enqueue(ctx, &job.Job{JobType: "test.retry"}); err != nil {
		t.Fatalf("enqueue retry job: %v", err)
	}
	if err := queue.Enqueue(ctx, &job.Job{JobType: "test.fail"}); err != nil {
		t.Fatalf("enqueue fail job: %v", err)
	}

	worker := job.NewWorker(queue, map[string]job.Handler{
		"test.retry": job.HandlerFunc(func(context.Context, *job.Job) error { return ai.ErrRateLimited }),
		"test.fail":  job.HandlerFunc(func(context.Context, *job.Job) error { return ai.ErrInvalidRequest }),
	}, "test-worker", time.Millisecond, testLogger())

	processAllJobs(t, worker)

	var retryStatus string
	var retryAttempts int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT status, attempts FROM scheduled_jobs WHERE job_type = 'test.retry'`,
	).Scan(&retryStatus, &retryAttempts); err != nil {
		t.Fatalf("load retry job: %v", err)
	}
	if retryStatus != job.StatusRetry || retryAttempts != 1 {
		t.Errorf("retry job = (%s, %d), want (retry, 1)", retryStatus, retryAttempts)
	}

	var failStatus string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT status FROM scheduled_jobs WHERE job_type = 'test.fail'`,
	).Scan(&failStatus); err != nil {
		t.Fatalf("load failed job: %v", err)
	}
	if failStatus != job.StatusFailed {
		t.Errorf("failed job status = %s, want failed", failStatus)
	}
}

func mustCreateDraft(t *testing.T, router *gin.Engine, token, content string) journalPayload {
	t.Helper()

	result := createJournalAPI(t, router, token, uuidString(), map[string]any{
		"content": content,
		"status":  "draft",
	})
	if result.Status != http.StatusCreated {
		t.Fatalf("create draft = %d: %s", result.Status, result.Body)
	}
	return decodeJSON[journalEnvelope](t, result.Body).Data
}

func uuidString() string {
	return uuid.NewString()
}
