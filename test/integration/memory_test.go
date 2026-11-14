//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type memoryPayload struct {
	ID                 string     `json:"id"`
	Content            string     `json:"content"`
	Category           *string    `json:"category"`
	Status             string     `json:"status"`
	Confidence         float64    `json:"confidence"`
	SourceCount        int        `json:"source_count"`
	SupersedesMemoryID *string    `json:"supersedes_memory_id"`
	LastConfirmedAt    *time.Time `json:"last_confirmed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type memorySourcePayload struct {
	ID            string     `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	SourceDate    *time.Time `json:"source_date"`
	Excerpt       *string    `json:"excerpt"`
	SourceDeleted bool       `json:"source_deleted"`
}

type memoryDetailPayload struct {
	memoryPayload
	Sources []memorySourcePayload `json:"sources"`
}

type memoryEnvelope struct {
	Data  memoryDetailPayload `json:"data"`
	Error errorPayload        `json:"error"`
}

type memoryListEnvelope struct {
	Data       []memoryPayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Error errorPayload `json:"error"`
}

func extractionReply(sourceID, content string) string {
	payload := map[string]any{
		"memories": []map[string]any{{
			"category":       "work",
			"content":        content,
			"confidence":     0.82,
			"certainty_type": "explicit",
			"source_ids":     []string{sourceID},
			"sensitivity":    "normal",
			"conflicts_with": []string{},
		}},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func newMemoryWorker(db *database.DB, extraction *memory.ExtractionService) *job.Worker {
	queue := job.NewQueue(db.Gorm)
	return job.NewWorker(queue, map[string]job.Handler{
		job.TypeJournalEmbedding: job.HandlerFunc(func(context.Context, *job.Job) error {
			return nil
		}),
		job.TypeJournalMemoryExtraction: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				JournalID string `json:"journal_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return err
			}
			return extraction.ProcessJournal(ctx, payload.JournalID)
		}),
	}, "memory-test-worker", time.Millisecond, testLogger())
}

func memoryAPIPath(memoryID string) string {
	return "/api/v1/memories/" + memoryID
}

func TestMemoryExtractionAndReviewFlow(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Late night",
		"content":    "I could not sleep well because of work and kept thinking about tomorrow.",
		"entry_date": "2026-11-14",
	})

	var pending int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_jobs
		 WHERE user_id = $1 AND job_type = $2 AND status = 'pending'`,
		user.User.ID, job.TypeJournalMemoryExtraction,
	).Scan(&pending); err != nil {
		t.Fatalf("count extraction jobs: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending extraction jobs = %d, want 1", pending)
	}

	repo := memory.NewRepository(db.Gorm)
	fake := &ai.FakeLLM{StructuredReply: extractionReply(journal.ID, "The user finds late-night work stressful.")}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", testLogger())
	processAllJobs(t, newMemoryWorker(db, extraction))

	list := do(t, router, http.MethodGet, "/api/v1/memories", requestOptions{token: user.AccessToken})
	if list.Status != http.StatusOK {
		t.Fatalf("list memories = %d: %s", list.Status, list.Body)
	}
	page := decodeJSON[memoryListEnvelope](t, list.Body)
	if len(page.Data) != 1 {
		t.Fatalf("memories = %d, want 1", len(page.Data))
	}
	created := page.Data[0]
	if created.Status != memory.StatusCandidate {
		t.Errorf("status = %s, want candidate", created.Status)
	}
	if created.SourceCount != 1 {
		t.Errorf("source_count = %d, want 1", created.SourceCount)
	}
	if created.Confidence < 0.8 || created.Confidence > 0.85 {
		t.Errorf("confidence = %f, want about 0.82", created.Confidence)
	}

	detail := do(t, router, http.MethodGet, memoryAPIPath(created.ID), requestOptions{token: user.AccessToken})
	if detail.Status != http.StatusOK {
		t.Fatalf("get memory = %d: %s", detail.Status, detail.Body)
	}
	loaded := decodeJSON[memoryEnvelope](t, detail.Body).Data
	if len(loaded.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(loaded.Sources))
	}
	if loaded.Sources[0].SourceType != "journal" || loaded.Sources[0].SourceID != journal.ID {
		t.Errorf("source = %+v, want the journal", loaded.Sources[0])
	}
	if loaded.Sources[0].SourceDate == nil || loaded.Sources[0].SourceDeleted {
		t.Errorf("source metadata = %+v", loaded.Sources[0])
	}

	// Re-running extraction must merge into the existing candidate instead of
	// creating a duplicate or a duplicate source.
	if err := extraction.ProcessJournal(ctx, journal.ID); err != nil {
		t.Fatalf("second extraction: %v", err)
	}
	var memories, sources int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM personal_memories WHERE user_id = $1`, user.User.ID,
	).Scan(&memories); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM memory_sources WHERE memory_id = $1`, created.ID,
	).Scan(&sources); err != nil {
		t.Fatalf("count sources: %v", err)
	}
	if memories != 1 || sources != 1 {
		t.Errorf("after re-extraction memories=%d sources=%d, want 1 and 1", memories, sources)
	}

	patched := do(t, router, http.MethodPatch, memoryAPIPath(created.ID), requestOptions{
		token: user.AccessToken,
		body: map[string]any{
			"content":  "The user finds late-night work stressful and hard to switch off.",
			"category": "work_routine",
		},
	})
	if patched.Status != http.StatusOK {
		t.Fatalf("patch memory = %d: %s", patched.Status, patched.Body)
	}
	updated := decodeJSON[memoryEnvelope](t, patched.Body).Data
	if updated.Status != memory.StatusCandidate {
		t.Errorf("status after patch = %s, want candidate", updated.Status)
	}
	if updated.Category == nil || *updated.Category != "work_routine" {
		t.Errorf("category = %v, want work_routine", updated.Category)
	}

	confirmed := do(t, router, http.MethodPost, memoryAPIPath(created.ID)+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "confirm"},
	})
	if confirmed.Status != http.StatusOK {
		t.Fatalf("confirm memory = %d: %s", confirmed.Status, confirmed.Body)
	}
	active := decodeJSON[memoryEnvelope](t, confirmed.Body).Data
	if active.Status != memory.StatusActive || active.LastConfirmedAt == nil {
		t.Errorf("confirmed memory = %+v", active)
	}

	activeList := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?status=active", requestOptions{token: user.AccessToken}).Body)
	if len(activeList.Data) != 1 {
		t.Errorf("active memories = %d, want 1", len(activeList.Data))
	}

	candidateList := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?status=candidate", requestOptions{token: user.AccessToken}).Body)
	if len(candidateList.Data) != 0 {
		t.Errorf("candidate memories = %d, want 0", len(candidateList.Data))
	}

	rejected := do(t, router, http.MethodPost, memoryAPIPath(created.ID)+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "reject"},
	})
	if rejected.Status != http.StatusOK {
		t.Fatalf("reject memory = %d: %s", rejected.Status, rejected.Body)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "work stressful", 5)
	if err != nil {
		t.Fatalf("retrieve memories: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("rejected memory appeared in retrieval: %+v", retrieved)
	}

	// A rejected memory can be confirmed again by the user.
	reconfirmed := do(t, router, http.MethodPost, memoryAPIPath(created.ID)+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "confirm"},
	})
	if reconfirmed.Status != http.StatusOK {
		t.Fatalf("reconfirm memory = %d: %s", reconfirmed.Status, reconfirmed.Body)
	}

	deleted := do(t, router, http.MethodDelete, memoryAPIPath(created.ID), requestOptions{token: user.AccessToken})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete memory = %d: %s", deleted.Status, deleted.Body)
	}
	if result := do(t, router, http.MethodGet, memoryAPIPath(created.ID), requestOptions{token: user.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", result.Status)
	}

	afterDelete := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories", requestOptions{token: user.AccessToken}).Body)
	if len(afterDelete.Data) != 0 {
		t.Errorf("memories after delete = %d, want 0", len(afterDelete.Data))
	}

	retrieved, err = service.Retrieve(ctx, user.User.ID, "work stressful", 5)
	if err != nil {
		t.Fatalf("retrieve after delete: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("deleted memory appeared in retrieval: %+v", retrieved)
	}
}

func TestMemoryExtractionRejectsForeignSources(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	owner := newTestUser(t, router)
	stranger := newTestUser(t, router)

	ownerJournal := mustCreateJournal(t, router, owner.AccessToken, map[string]any{
		"content":    "I want to become more consistent with morning walks.",
		"entry_date": "2026-11-12",
	})
	strangerJournal := mustCreateJournal(t, router, stranger.AccessToken, map[string]any{
		"content":    "Private note that must not become someone else's memory.",
		"entry_date": "2026-11-12",
	})

	repo := memory.NewRepository(db.Gorm)
	fake := &ai.FakeLLM{StructuredReply: extractionReply(strangerJournal.ID, "The user wants a morning walking routine.")}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", testLogger())

	if err := extraction.ProcessJournal(ctx, ownerJournal.ID); err != nil {
		t.Fatalf("foreign-source extraction: %v", err)
	}

	var count int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM personal_memories WHERE user_id = $1`, owner.User.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 0 {
		t.Fatalf("memories from a foreign source = %d, want 0", count)
	}

	fake.StructuredReply = extractionReply(ownerJournal.ID, "The user wants a morning walking routine.")
	if err := extraction.ProcessJournal(ctx, ownerJournal.ID); err != nil {
		t.Fatalf("valid extraction: %v", err)
	}

	list := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories", requestOptions{token: owner.AccessToken}).Body)
	if len(list.Data) != 1 {
		t.Fatalf("memories = %d, want 1", len(list.Data))
	}
	memoryID := list.Data[0].ID

	if result := do(t, router, http.MethodGet, memoryAPIPath(memoryID), requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger get = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodPatch, memoryAPIPath(memoryID), requestOptions{
		token: stranger.AccessToken,
		body:  map[string]any{"content": "tampered"},
	}); result.Status != http.StatusNotFound {
		t.Errorf("stranger patch = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodPost, memoryAPIPath(memoryID)+"/feedback", requestOptions{
		token: stranger.AccessToken,
		body:  map[string]any{"action": "confirm"},
	}); result.Status != http.StatusNotFound {
		t.Errorf("stranger feedback = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodDelete, memoryAPIPath(memoryID), requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger delete = %d, want 404", result.Status)
	}

	strangerList := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories", requestOptions{token: stranger.AccessToken}).Body)
	if len(strangerList.Data) != 0 {
		t.Errorf("stranger memories = %d, want 0", len(strangerList.Data))
	}
}

func TestMemoryListPaginationAndFilters(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "A source entry for seeded memories.",
		"entry_date": "2026-11-11",
	})
	repo := memory.NewRepository(db.Gorm)
	base := time.Now().UTC().Add(-time.Hour)

	seed := []struct {
		content  string
		category string
		status   string
	}{
		{content: "The user values quiet mornings.", category: "routine", status: memory.StatusCandidate},
		{content: "The user works best in the morning.", category: "work", status: memory.StatusActive},
		{content: "The user wants to read more books.", category: "goal", status: memory.StatusActive},
	}

	for index, item := range seed {
		category := item.category
		confidence := 0.7
		record := memory.Memory{
			ID:          uuid.NewString(),
			UserID:      user.User.ID,
			Content:     item.content,
			Category:    &category,
			Status:      item.status,
			Confidence:  &confidence,
			SourceCount: 1,
			CreatedAt:   base.Add(time.Duration(index) * time.Minute),
			UpdatedAt:   base.Add(time.Duration(index) * time.Minute),
		}
		if err := repo.Create(ctx, &record, journal.ID); err != nil {
			t.Fatalf("seed memory: %v", err)
		}
	}

	first := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?limit=2", requestOptions{token: user.AccessToken}).Body)
	if len(first.Data) != 2 || !first.Pagination.HasMore || first.Pagination.NextCursor == nil {
		t.Fatalf("first page = %+v", first)
	}

	second := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?limit=2&cursor="+*first.Pagination.NextCursor,
		requestOptions{token: user.AccessToken}).Body)
	if len(second.Data) != 1 {
		t.Errorf("second page size = %d, want 1", len(second.Data))
	}

	active := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?status=active", requestOptions{token: user.AccessToken}).Body)
	if len(active.Data) != 2 {
		t.Errorf("active memories = %d, want 2", len(active.Data))
	}

	goals := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?category=goal", requestOptions{token: user.AccessToken}).Body)
	if len(goals.Data) != 1 || goals.Data[0].Category == nil || *goals.Data[0].Category != "goal" {
		t.Errorf("goal memories = %+v", goals.Data)
	}

	if result := do(t, router, http.MethodGet,
		"/api/v1/memories?status=deleted", requestOptions{token: user.AccessToken}); result.Status != http.StatusBadRequest {
		t.Errorf("deleted status filter = %d, want 400", result.Status)
	}
}

func TestCompanionUsesConfirmedMemories(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "A source entry for memory context.",
		"entry_date": "2026-11-13",
	})
	repo := memory.NewRepository(db.Gorm)
	category := "work"
	confidence := 0.9
	seed := memory.Memory{
		ID:              uuid.NewString(),
		UserID:          user.User.ID,
		Content:         "The user finds work stressful.",
		Category:        &category,
		Status:          memory.StatusActive,
		Confidence:      &confidence,
		SourceCount:     1,
		LastConfirmedAt: ptrTime(time.Now().UTC()),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, &seed, journal.ID); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	conversation := mustCreateConversation(t, router, user.AccessToken, "Memory context")
	sent := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "I feel stressed about work.")
	if sent.Status != http.StatusOK {
		t.Fatalf("send message = %d: %s", sent.Status, sent.Body)
	}
	payload := decodeJSON[sendEnvelope](t, sent.Body).Data

	var metadata []byte
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT metadata FROM messages WHERE id = $1`, payload.AssistantMessage.ID,
	).Scan(&metadata); err != nil {
		t.Fatalf("load assistant metadata: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(metadata, &parsed); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if used, ok := parsed["memories_used"].(float64); !ok || used < 1 {
		t.Errorf("memories_used = %v, want at least 1", parsed["memories_used"])
	}
}

func TestDeletedJournalDowngradesMemory(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "The only source for this memory will be deleted.",
		"entry_date": "2026-11-10",
	})
	repo := memory.NewRepository(db.Gorm)
	category := "work"
	confidence := 0.8
	seed := memory.Memory{
		ID:          uuid.NewString(),
		UserID:      user.User.ID,
		Content:     "The user finds work stressful.",
		Category:    &category,
		Status:      memory.StatusActive,
		Confidence:  &confidence,
		SourceCount: 1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(ctx, &seed, journal.ID); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	deleted := do(t, router, http.MethodDelete, journalAPIPath(journal.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete journal = %d: %s", deleted.Status, deleted.Body)
	}

	detail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(seed.ID), requestOptions{token: user.AccessToken}).Body).Data
	if detail.Status != memory.StatusCandidate {
		t.Errorf("status = %s, want candidate after losing the last source", detail.Status)
	}
	if detail.SourceCount != 0 {
		t.Errorf("source_count = %d, want 0", detail.SourceCount)
	}
	if len(detail.Sources) != 1 || !detail.Sources[0].SourceDeleted {
		t.Errorf("sources = %+v, want one deleted source", detail.Sources)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "work stressful", 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("memory without sources appeared in retrieval: %+v", retrieved)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
