//go:build integration

package integration_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type searchResultPayload struct {
	Type        string  `json:"type"`
	ID          string  `json:"id"`
	ParentID    *string `json:"parent_id"`
	Title       *string `json:"title"`
	Snippet     string  `json:"snippet"`
	Date        string  `json:"date"`
	Status      *string `json:"status"`
	Sensitivity *string `json:"sensitivity"`
	Score       float64 `json:"score"`
}

type searchEnvelope struct {
	Data struct {
		Query   string                `json:"query"`
		Results []searchResultPayload `json:"results"`
		Counts  map[string]int        `json:"counts"`
		Limit   int                   `json:"limit"`
	} `json:"data"`
	Error errorPayload `json:"error"`
}

func TestUnifiedSearch(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)
	stranger := newTestUser(t, router)

	entry := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Quasar planning",
		"content":    "I spent the afternoon on quasar planning and felt focused.",
		"entry_date": "2026-11-21",
	})

	memoryID := uuid.NewString()
	if _, err := db.SQL.ExecContext(t.Context(),
		`INSERT INTO personal_memories
		 (id, user_id, content, category, sensitivity, status, confidence, source_count, created_at, updated_at)
		 VALUES ($1, $2, $3, 'work', 'normal', 'active', 0.80, 1, now(), now())`,
		memoryID, user.User.ID,
		"The quasar project gives the user a sense of focus.",
	); err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	insightID := uuid.NewString()
	if _, err := db.SQL.ExecContext(t.Context(),
		`INSERT INTO insights
		 (id, user_id, title, summary, observation, interpretation, hypothesis,
		  confidence, confidence_label, reflection_question, suggested_direction,
		  generation_type, created_at, updated_at)
		 VALUES ($1, $2, 'Quasar focus', 'Quasar work appears meaningful.',
		         'The entry mentions quasar planning.', 'Focus may come from meaningful work.',
		         NULL, 0.70, 'moderate', 'What protects this focus?', 'maintain',
		         'on_demand', now(), now())`,
		insightID, user.User.ID,
	); err != nil {
		t.Fatalf("insert insight: %v", err)
	}

	analysisID := uuid.NewString()
	if _, err := db.SQL.ExecContext(t.Context(),
		`INSERT INTO journal_analyses
		 (id, user_id, journal_id, journal_version, job_id, summary, observation,
		  interpretation, hypothesis, confidence, confidence_label,
		  reflection_question, suggested_direction, evidence_note, model,
		  prompt_version, created_at)
		 VALUES ($1, $2, $3, 1, $4, 'Quasar planning supports focus.',
		         'The entry describes quasar planning.', 'Meaningful work may support focus.',
		         NULL, 0.68, 'moderate', 'What would protect this time?', 'maintain',
		         'The entry connects quasar planning with feeling focused.',
		         'fake-chat', 'journal_analysis/v1', now())`,
		analysisID, user.User.ID, entry.ID, uuid.NewString(),
	); err != nil {
		t.Fatalf("insert analysis: %v", err)
	}

	all := do(t, router, http.MethodGet, "/api/v1/search?q=quasar", requestOptions{token: user.AccessToken})
	if all.Status != http.StatusOK {
		t.Fatalf("search = %d: %s", all.Status, all.Body)
	}
	response := decodeJSON[searchEnvelope](t, all.Body).Data
	if len(response.Results) != 4 {
		t.Fatalf("search results = %d, want 4: %s", len(response.Results), all.Body)
	}
	if response.Counts["journals"] != 1 || response.Counts["memories"] != 1 ||
		response.Counts["insights"] != 1 || response.Counts["analyses"] != 1 {
		t.Errorf("search counts = %+v", response.Counts)
	}
	if response.Limit != 10 {
		t.Errorf("search limit = %d, want 10", response.Limit)
	}

	seen := map[string]bool{}
	for _, result := range response.Results {
		seen[result.Type] = true
		if result.Snippet == "" || len([]rune(result.Snippet)) > 230 {
			t.Errorf("result snippet = %q", result.Snippet)
		}
		if result.Score <= 0 {
			t.Errorf("result score = %f", result.Score)
		}
	}
	for _, want := range []string{"journal", "memory", "insight", "analysis"} {
		if !seen[want] {
			t.Errorf("search is missing type %q", want)
		}
	}

	filtered := decodeJSON[searchEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/search?q=quasar&types=journals,memories&limit=1",
		requestOptions{token: user.AccessToken}).Body).Data
	if len(filtered.Results) != 2 || filtered.Counts["journals"] != 1 || filtered.Counts["memories"] != 1 {
		t.Errorf("filtered search = %+v", filtered)
	}
	for _, result := range filtered.Results {
		if result.Type != "journal" && result.Type != "memory" {
			t.Errorf("filtered result type = %s", result.Type)
		}
	}

	none := decodeJSON[searchEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/search?q=zzzznotfound", requestOptions{token: user.AccessToken}).Body).Data
	if len(none.Results) != 0 {
		t.Errorf("empty search results = %+v", none.Results)
	}

	strangerSearch := decodeJSON[searchEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/search?q=quasar", requestOptions{token: stranger.AccessToken}).Body).Data
	if len(strangerSearch.Results) != 0 {
		t.Errorf("stranger search results = %d, want 0", len(strangerSearch.Results))
	}

	if result := do(t, router, http.MethodGet, "/api/v1/search?q=a", requestOptions{token: user.AccessToken}); result.Status != http.StatusBadRequest {
		t.Errorf("short query = %d, want 400", result.Status)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/search?q=quasar&types=unknown", requestOptions{token: user.AccessToken}); result.Status != http.StatusBadRequest {
		t.Errorf("invalid type = %d, want 400", result.Status)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/search?q=quasar&limit=21", requestOptions{token: user.AccessToken}); result.Status != http.StatusBadRequest {
		t.Errorf("invalid limit = %d, want 400", result.Status)
	}

	if result := do(t, router, http.MethodDelete, "/api/v1/journals/"+entry.ID, requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
	}); result.Status != http.StatusNoContent {
		t.Fatalf("delete journal = %d: %s", result.Status, result.Body)
	}
	afterDelete := decodeJSON[searchEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/search?q=quasar", requestOptions{token: user.AccessToken}).Body).Data
	if afterDelete.Counts["journals"] != 0 || afterDelete.Counts["analyses"] != 0 {
		t.Errorf("search after journal delete = %+v", afterDelete.Counts)
	}
	if afterDelete.Counts["memories"] != 1 || afterDelete.Counts["insights"] != 1 {
		t.Errorf("derived search after journal delete = %+v", afterDelete.Counts)
	}

	for _, result := range afterDelete.Results {
		if strings.Contains(result.Snippet, "password_hash") {
			t.Error("search snippet contains secret metadata")
		}
	}
}
