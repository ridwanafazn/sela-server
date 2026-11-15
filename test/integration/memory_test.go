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
	Sources       []memorySourcePayload `json:"sources"`
	ConflictsWith []string              `json:"conflicts_with"`
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
	return extractionReplyWithConflicts(sourceID, content, nil)
}

func extractionReplyWithConflicts(sourceID, content string, conflicts []string) string {
	if conflicts == nil {
		conflicts = []string{}
	}
	payload := map[string]any{
		"memories": []map[string]any{{
			"category":       "work",
			"content":        content,
			"confidence":     0.82,
			"certainty_type": "explicit",
			"source_ids":     []string{sourceID},
			"sensitivity":    "normal",
			"conflicts_with": conflicts,
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
		job.TypeMessageMemoryExtraction: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			var payload struct {
				MessageID string `json:"message_id"`
			}
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return err
			}
			return extraction.ProcessMessage(ctx, payload.MessageID)
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
		if err := repo.Create(ctx, &record, journalSourceRef(journal.ID), nil); err != nil {
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
	if err := repo.Create(ctx, &seed, journalSourceRef(journal.ID), nil); err != nil {
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
	if err := repo.Create(ctx, &seed, journalSourceRef(journal.ID), nil); err != nil {
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

func TestMemoryCorrectionSupersedesOriginal(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Work has been stressful lately.",
		"entry_date": "2026-11-09",
	})
	repo := memory.NewRepository(db.Gorm)
	category := "work"
	confidence := 0.8
	original := memory.Memory{
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
	if err := repo.Create(ctx, &original, journalSourceRef(journal.ID), nil); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	corrected := do(t, router, http.MethodPost, memoryAPIPath(original.ID)+"/feedback", requestOptions{
		token: user.AccessToken,
		body: map[string]any{
			"action":            "correct",
			"corrected_content": "The user finds work stressful but manageable.",
		},
	})
	if corrected.Status != http.StatusOK {
		t.Fatalf("correct memory = %d: %s", corrected.Status, corrected.Body)
	}
	replacement := decodeJSON[memoryEnvelope](t, corrected.Body).Data
	if replacement.ID == original.ID {
		t.Fatal("correction did not create a replacement memory")
	}
	if replacement.Status != memory.StatusActive {
		t.Errorf("replacement status = %s, want active", replacement.Status)
	}
	if replacement.SupersedesMemoryID == nil || *replacement.SupersedesMemoryID != original.ID {
		t.Errorf("supersedes_memory_id = %v, want %s", replacement.SupersedesMemoryID, original.ID)
	}
	if len(replacement.Sources) != 1 || replacement.Sources[0].SourceID != journal.ID {
		t.Errorf("replacement sources = %+v", replacement.Sources)
	}

	originalDetail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(original.ID), requestOptions{token: user.AccessToken}).Body).Data
	if originalDetail.Status != memory.StatusSuperseded {
		t.Errorf("original status = %s, want superseded", originalDetail.Status)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "work stressful", 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(retrieved) != 1 || retrieved[0].ID != replacement.ID {
		t.Errorf("retrieval = %+v, want only the replacement", retrieved)
	}
}

func TestMemoryExtractionPersistsConflicts(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	sourceJournal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "I prefer working in the evening.",
		"entry_date": "2026-11-08",
	})
	repo := memory.NewRepository(db.Gorm)
	category := "routine"
	confidence := 0.7
	existing := memory.Memory{
		ID:          uuid.NewString(),
		UserID:      user.User.ID,
		Content:     "The user prefers working in the morning.",
		Category:    &category,
		Status:      memory.StatusActive,
		Confidence:  &confidence,
		SourceCount: 1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(ctx, &existing, journalSourceRef(sourceJournal.ID), nil); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	conflictJournal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Evening work suits me better than mornings.",
		"entry_date": "2026-11-10",
	})
	fake := &ai.FakeLLM{StructuredReply: extractionReplyWithConflicts(
		conflictJournal.ID,
		"The user prefers working in the evening.",
		[]string{existing.ID},
	)}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", testLogger())
	if err := extraction.ProcessJournal(ctx, conflictJournal.ID); err != nil {
		t.Fatalf("extract conflict: %v", err)
	}

	list := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories?status=candidate", requestOptions{token: user.AccessToken}).Body)
	if len(list.Data) != 1 {
		t.Fatalf("candidate memories = %d, want 1", len(list.Data))
	}
	candidate := list.Data[0]

	candidateDetail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(candidate.ID), requestOptions{token: user.AccessToken}).Body).Data
	if len(candidateDetail.ConflictsWith) != 1 || candidateDetail.ConflictsWith[0] != existing.ID {
		t.Errorf("candidate conflicts = %+v, want %s", candidateDetail.ConflictsWith, existing.ID)
	}

	existingDetail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(existing.ID), requestOptions{token: user.AccessToken}).Body).Data
	if len(existingDetail.ConflictsWith) != 1 || existingDetail.ConflictsWith[0] != candidate.ID {
		t.Errorf("existing conflicts = %+v, want %s", existingDetail.ConflictsWith, candidate.ID)
	}
}

func TestMessageMemoryExtraction(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	conversation := mustCreateConversation(t, router, user.AccessToken, "Message extraction")
	sent := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "I prefer walking after lunch to reset.")
	if sent.Status != http.StatusOK {
		t.Fatalf("send message = %d: %s", sent.Status, sent.Body)
	}
	payload := decodeJSON[sendEnvelope](t, sent.Body).Data

	var pending int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_jobs
		 WHERE user_id = $1 AND job_type = $2 AND status = 'pending'`,
		user.User.ID, job.TypeMessageMemoryExtraction,
	).Scan(&pending); err != nil {
		t.Fatalf("count message extraction jobs: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending message extraction jobs = %d, want 1 (user message only)", pending)
	}

	var payloadMessageID string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT payload->>'message_id' FROM scheduled_jobs
		 WHERE user_id = $1 AND job_type = $2`,
		user.User.ID, job.TypeMessageMemoryExtraction,
	).Scan(&payloadMessageID); err != nil {
		t.Fatalf("load message extraction payload: %v", err)
	}
	if payloadMessageID != payload.UserMessage.ID {
		t.Errorf("job message_id = %s, want %s", payloadMessageID, payload.UserMessage.ID)
	}

	repo := memory.NewRepository(db.Gorm)
	fake := &ai.FakeLLM{StructuredReply: extractionReply(payload.UserMessage.ID, "The user prefers a walk after lunch.")}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", testLogger())
	processAllJobs(t, newMemoryWorker(db, extraction))

	list := decodeJSON[memoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories", requestOptions{token: user.AccessToken}).Body)
	if len(list.Data) != 1 {
		t.Fatalf("memories = %d, want 1", len(list.Data))
	}
	memoryID := list.Data[0].ID

	detail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(memoryID), requestOptions{token: user.AccessToken}).Body).Data
	if len(detail.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(detail.Sources))
	}
	if detail.Sources[0].SourceType != "message" || detail.Sources[0].SourceID != payload.UserMessage.ID {
		t.Errorf("source = %+v, want the user message", detail.Sources[0])
	}

	deleted := do(t, router, http.MethodDelete, "/api/v1/conversations/"+conversation.ID, requestOptions{token: user.AccessToken})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete conversation = %d: %s", deleted.Status, deleted.Body)
	}

	afterDelete := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(memoryID), requestOptions{token: user.AccessToken}).Body).Data
	if afterDelete.SourceCount != 0 || afterDelete.Status != memory.StatusCandidate {
		t.Errorf("memory after conversation delete = %+v", afterDelete)
	}
	if len(afterDelete.Sources) != 1 || !afterDelete.Sources[0].SourceDeleted {
		t.Errorf("sources after conversation delete = %+v", afterDelete.Sources)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "walk lunch", 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("memory from a deleted conversation appeared in retrieval: %+v", retrieved)
	}
}

func TestMessageMemoryExtractionRejectsForeignSource(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	owner := newTestUser(t, router)
	stranger := newTestUser(t, router)

	ownerConversation := mustCreateConversation(t, router, owner.AccessToken, "Owner")
	ownerSent := sendMessageAPI(t, router, owner.AccessToken, ownerConversation.ID, uuid.NewString(), "I want to read more.")
	if ownerSent.Status != http.StatusOK {
		t.Fatalf("owner send = %d: %s", ownerSent.Status, ownerSent.Body)
	}
	ownerMessage := decodeJSON[sendEnvelope](t, ownerSent.Body).Data.UserMessage

	strangerConversation := mustCreateConversation(t, router, stranger.AccessToken, "Stranger")
	strangerSent := sendMessageAPI(t, router, stranger.AccessToken, strangerConversation.ID, uuid.NewString(), "Private message.")
	if strangerSent.Status != http.StatusOK {
		t.Fatalf("stranger send = %d: %s", strangerSent.Status, strangerSent.Body)
	}
	strangerMessage := decodeJSON[sendEnvelope](t, strangerSent.Body).Data.UserMessage

	repo := memory.NewRepository(db.Gorm)
	fake := &ai.FakeLLM{StructuredReply: extractionReply(strangerMessage.ID, "The user wants to read more.")}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", testLogger())
	if err := extraction.ProcessMessage(ctx, ownerMessage.ID); err != nil {
		t.Fatalf("foreign-source extraction: %v", err)
	}

	var count int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM personal_memories WHERE user_id = $1`, owner.User.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 0 {
		t.Errorf("memories from a foreign message = %d, want 0", count)
	}
}

func TestJournalDraftDowngradesMemory(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "An active entry that will become a draft.",
		"entry_date": "2026-11-07",
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
	if err := repo.Create(ctx, &seed, journalSourceRef(journal.ID), nil); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	drafted := do(t, router, http.MethodPatch, journalAPIPath(journal.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body:    map[string]any{"status": "draft"},
	})
	if drafted.Status != http.StatusOK {
		t.Fatalf("draft journal = %d: %s", drafted.Status, drafted.Body)
	}

	detail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(seed.ID), requestOptions{token: user.AccessToken}).Body).Data
	if detail.Status != memory.StatusCandidate || detail.SourceCount != 0 {
		t.Errorf("memory after draft = %+v, want candidate with zero sources", detail)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "work stressful", 5)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("memory sourced only by a draft appeared in retrieval: %+v", retrieved)
	}
}

func TestPartialSourceDeletionKeepsMemoryActive(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	first := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "First supporting entry.",
		"entry_date": "2026-11-06",
	})
	second := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Second supporting entry.",
		"entry_date": "2026-11-05",
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
	if err := repo.Create(ctx, &seed, journalSourceRef(first.ID), nil); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if err := repo.AddSource(ctx, seed.ID, journalSourceRef(second.ID), time.Now().UTC()); err != nil {
		t.Fatalf("add source: %v", err)
	}

	deleted := do(t, router, http.MethodDelete, journalAPIPath(first.ID), requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete first journal = %d: %s", deleted.Status, deleted.Body)
	}

	detail := decodeJSON[memoryEnvelope](t, do(t, router, http.MethodGet,
		memoryAPIPath(seed.ID), requestOptions{token: user.AccessToken}).Body).Data
	if detail.Status != memory.StatusActive {
		t.Errorf("status = %s, want active while one source remains", detail.Status)
	}
	if detail.SourceCount != 1 {
		t.Errorf("source_count = %d, want 1", detail.SourceCount)
	}
}

func journalSourceRef(journalID string) memory.SourceRef {
	return memory.SourceRef{JournalID: &journalID}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
