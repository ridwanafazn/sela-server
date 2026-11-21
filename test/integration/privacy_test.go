//go:build integration

package integration_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/modules/privacy"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type privacyEnvelope struct {
	Data struct {
		AIContextMode           string  `json:"ai_context_mode"`
		MemoryExtractionEnabled bool    `json:"memory_extraction_enabled"`
		SensitiveMemoryGranted  bool    `json:"sensitive_memory_granted"`
		AdultConfirmed          bool    `json:"adult_confirmed"`
		TermsVersion            *string `json:"terms_version"`
		PrivacyVersion          *string `json:"privacy_version"`
	} `json:"data"`
	Error errorPayload `json:"error"`
}

func privacyServiceForTest(db *database.DB) *privacy.Service {
	return privacy.NewService(privacy.NewRepository(db.Gorm), testLogger())
}

func TestPrivacyPreferencesAndEnforcement(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)
	stranger := newTestUser(t, router)

	initial := decodeJSON[privacyEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me/privacy", requestOptions{
		token: user.AccessToken,
	}).Body).Data
	if initial.AIContextMode != privacy.ContextJournalsAndMemories || !initial.MemoryExtractionEnabled {
		t.Fatalf("initial privacy = %+v", initial)
	}

	updated := do(t, router, http.MethodPut, "/api/v1/me/privacy", requestOptions{
		token: user.AccessToken,
		body: map[string]any{
			"ai_context_mode":           privacy.ContextNone,
			"memory_extraction_enabled": false,
		},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update privacy = %d: %s", updated.Status, updated.Body)
	}
	payload := decodeJSON[privacyEnvelope](t, updated.Body).Data
	if payload.AIContextMode != privacy.ContextNone || payload.MemoryExtractionEnabled {
		t.Errorf("updated privacy = %+v", payload)
	}

	if result := do(t, router, http.MethodPut, "/api/v1/me/privacy", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"ai_context_mode": "invalid"},
	}); result.Status != http.StatusBadRequest {
		t.Errorf("invalid context mode = %d, want 400", result.Status)
	}

	strangerPrivacy := decodeJSON[privacyEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me/privacy", requestOptions{
		token: stranger.AccessToken,
	}).Body).Data
	if strangerPrivacy.AIContextMode != privacy.ContextJournalsAndMemories || !strangerPrivacy.MemoryExtractionEnabled {
		t.Errorf("stranger privacy = %+v, want defaults", strangerPrivacy)
	}

	// Memory extraction is skipped while the preference is off.
	first := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Quasar planning at work feels meaningful and I want to keep the momentum going.",
		"entry_date": "2026-11-21",
	})
	fake := &ai.FakeLLM{StructuredReply: extractionReply(first.ID, "The user finds quasar planning meaningful.")}
	extraction := memory.NewExtractionService(
		memory.NewRepository(db.Gorm), fake, "fake-chat", "fake", nil, testLogger(),
	).WithPrivacy(privacyServiceForTest(db))
	processAllJobs(t, newMemoryWorker(db, extraction))
	var count int
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*) FROM personal_memories WHERE user_id = $1`, user.User.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count memories after disabled extraction: %v", err)
	}
	if count != 0 {
		t.Errorf("memories after disabled extraction = %d, want 0", count)
	}

	if result := do(t, router, http.MethodPut, "/api/v1/me/privacy", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"memory_extraction_enabled": true},
	}); result.Status != http.StatusOK {
		t.Fatalf("re-enable memory extraction = %d: %s", result.Status, result.Body)
	}

	second := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Quasar planning went well today and I want to protect the focus time that made it possible.",
		"entry_date": "2026-11-22",
	})
	fake.StructuredReply = extractionReply(second.ID, "The user protects focus time for quasar planning.")
	processAllJobs(t, newMemoryWorker(db, extraction))
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*) FROM personal_memories WHERE user_id = $1`, user.User.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count memories after enabled extraction: %v", err)
	}
	if count != 1 {
		t.Errorf("memories after enabled extraction = %d, want 1", count)
	}

	// Insights do not use personal context when the mode is none.
	if result := do(t, router, http.MethodPut, "/api/v1/me/privacy", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"ai_context_mode": privacy.ContextNone},
	}); result.Status != http.StatusOK {
		t.Fatalf("set context none = %d: %s", result.Status, result.Body)
	}

	accepted := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"period":     "custom",
		"start_date": "2026-11-01",
		"end_date":   "2026-11-30",
	})
	if accepted.Status != http.StatusAccepted {
		t.Fatalf("request insight with context none = %d: %s", accepted.Status, accepted.Body)
	}
	acceptedPayload := decodeJSON[insightAcceptedEnvelope](t, accepted.Body).Data

	insightGeneration := insight.NewGenerationService(
		insight.NewRepository(db.Gorm), &ai.FakeLLM{StructuredReply: insightReply(
			[]string{first.ID, second.ID}, 0.7,
		)}, "fake-chat", "fake", nil, testLogger(),
	).WithPrivacy(privacyServiceForTest(db))
	processAllJobs(t, newInsightWorker(db, insightGeneration))

	completed := decodeJSON[insightJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insight-jobs/"+acceptedPayload.JobID,
		requestOptions{token: user.AccessToken}).Body).Data
	if completed.Status != "completed" || completed.Outcome == nil || *completed.Outcome != "insufficient_evidence" {
		t.Errorf("insight job with context none = %+v", completed)
	}
	if completed.InsightID != nil {
		t.Errorf("insight with context none has insight_id %s", *completed.InsightID)
	}
}
