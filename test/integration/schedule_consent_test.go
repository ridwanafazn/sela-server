//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type schedulePayload struct {
	Enabled       bool       `json:"enabled"`
	Cadence       *string    `json:"cadence"`
	Timezone      string     `json:"timezone"`
	NextRunAt     *time.Time `json:"next_run_at"`
	LastRunAt     *time.Time `json:"last_run_at"`
	BudgetBlocked bool       `json:"budget_blocked"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

type scheduleEnvelope struct {
	Data  schedulePayload `json:"data"`
	Error errorPayload    `json:"error"`
}

type consentPayload struct {
	Scope     string     `json:"scope"`
	Granted   bool       `json:"granted"`
	GrantedAt *time.Time `json:"granted_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type consentEnvelope struct {
	Data  consentPayload `json:"data"`
	Error errorPayload   `json:"error"`
}

type sensitiveMemoryPayload struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Sensitivity string `json:"sensitivity"`
	Status      string `json:"status"`
}

type sensitiveMemoryListEnvelope struct {
	Data  []sensitiveMemoryPayload `json:"data"`
	Error errorPayload             `json:"error"`
}

func sensitiveExtractionReply(sourceID, content string) string {
	payload := map[string]any{
		"memories": []map[string]any{{
			"category":       "health",
			"content":        content,
			"confidence":     0.9,
			"certainty_type": "explicit",
			"source_ids":     []string{sourceID},
			"sensitivity":    "sensitive",
			"conflicts_with": []string{},
		}},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func TestInsightScheduleLifecycleAndFanOut(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	initial := do(t, router, http.MethodGet, "/api/v1/insight-schedule", requestOptions{token: user.AccessToken})
	if initial.Status != http.StatusOK {
		t.Fatalf("get initial schedule = %d: %s", initial.Status, initial.Body)
	}
	if view := decodeJSON[scheduleEnvelope](t, initial.Body).Data; view.Enabled || view.Cadence != nil {
		t.Fatalf("initial schedule = %+v, want disabled", view)
	}

	enabled := do(t, router, http.MethodPut, "/api/v1/insight-schedule", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"enabled": true, "cadence": "weekly"},
	})
	if enabled.Status != http.StatusOK {
		t.Fatalf("enable schedule = %d: %s", enabled.Status, enabled.Body)
	}
	view := decodeJSON[scheduleEnvelope](t, enabled.Body).Data
	if !view.Enabled || view.Cadence == nil || *view.Cadence != insight.CadenceWeekly {
		t.Fatalf("enabled schedule = %+v", view)
	}
	if view.NextRunAt == nil || view.Timezone == "" {
		t.Errorf("enabled schedule missing next run or timezone: %+v", view)
	}

	invalid := do(t, router, http.MethodPut, "/api/v1/insight-schedule", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"enabled": true, "cadence": "daily"},
	})
	if invalid.Status != http.StatusBadRequest {
		t.Errorf("invalid cadence = %d, want 400", invalid.Status)
	}

	// Force a due run in a known past week so fan-out is deterministic.
	if _, err := db.SQL.ExecContext(ctx,
		`UPDATE insight_schedules SET next_run_at = '2026-11-16T01:00:00+07:00' WHERE user_id = $1`,
		user.User.ID,
	); err != nil {
		t.Fatalf("force due schedule: %v", err)
	}

	first := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Work pressure affected my sleep last week.",
		"entry_date": "2026-11-10",
	})
	second := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "I protected one evening with my family.",
		"entry_date": "2026-11-12",
	})

	repo := insight.NewRepository(db.Gorm)
	queue := job.NewQueue(db.Gorm)
	scheduleService := insight.NewScheduleService(repo, queue, nil, testLogger())
	result, err := scheduleService.FanOutDue(ctx)
	if err != nil {
		t.Fatalf("fan out: %v", err)
	}
	if result.Due != 1 || result.Enqueued != 1 {
		t.Fatalf("fan-out result = %+v, want one enqueued job", result)
	}

	var payload insight.JobPayload
	var rawPayload []byte
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT payload FROM scheduled_jobs
		 WHERE user_id = $1 AND job_type = $2
		 ORDER BY created_at DESC LIMIT 1`,
		user.User.ID, job.TypeInsightGenerate,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled job payload: %v", err)
	}
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("decode scheduled job payload: %v", err)
	}
	if payload.GenerationType != insight.CadenceWeekly ||
		payload.ScheduleID == "" ||
		payload.StartDate != "2026-11-09" ||
		payload.EndDate != "2026-11-15" {
		t.Errorf("scheduled payload = %+v", payload)
	}

	// Re-running fan-out for the same period must not create a second job.
	if _, err := db.SQL.ExecContext(ctx,
		`UPDATE insight_schedules SET next_run_at = '2026-11-16T01:00:00+07:00' WHERE user_id = $1`,
		user.User.ID,
	); err != nil {
		t.Fatalf("reset due schedule: %v", err)
	}
	replay, err := scheduleService.FanOutDue(ctx)
	if err != nil {
		t.Fatalf("replay fan-out: %v", err)
	}
	if replay.Due != 1 || replay.Existing != 1 || replay.Enqueued != 0 {
		t.Errorf("replay fan-out = %+v, want one existing job", replay)
	}

	fake := &ai.FakeLLM{StructuredReply: insightReply([]string{first.ID, second.ID}, 0.75)}
	generation := insight.NewGenerationService(repo, fake, "fake-chat", "fake", nil, testLogger())
	processAllJobs(t, newInsightWorker(db, generation))

	var generationType, periodStart, periodEnd string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT generation_type, period_start::text, period_end::text
		 FROM insights WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		user.User.ID,
	).Scan(&generationType, &periodStart, &periodEnd); err != nil {
		t.Fatalf("load scheduled insight: %v", err)
	}
	if generationType != insight.GenerationWeekly || periodStart != "2026-11-09" || periodEnd != "2026-11-15" {
		t.Errorf("scheduled insight = (%s, %s, %s)", generationType, periodStart, periodEnd)
	}

	disabled := do(t, router, http.MethodDelete, "/api/v1/insight-schedule", requestOptions{token: user.AccessToken})
	if disabled.Status != http.StatusOK {
		t.Fatalf("disable schedule = %d: %s", disabled.Status, disabled.Body)
	}
	if view := decodeJSON[scheduleEnvelope](t, disabled.Body).Data; view.Enabled {
		t.Errorf("schedule is still enabled: %+v", view)
	}
}

func TestSensitiveMemoryConsentLifecycle(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()
	user := newTestUser(t, router)

	initial := do(t, router, http.MethodGet, "/api/v1/memory-consent", requestOptions{token: user.AccessToken})
	if initial.Status != http.StatusOK {
		t.Fatalf("get initial consent = %d: %s", initial.Status, initial.Body)
	}
	if consent := decodeJSON[consentEnvelope](t, initial.Body).Data; consent.Granted {
		t.Fatalf("initial consent = %+v, want not granted", consent)
	}

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "A sensitive health detail I chose to share.",
		"entry_date": "2026-11-14",
	})
	repo := memory.NewRepository(db.Gorm)
	fake := &ai.FakeLLM{StructuredReply: sensitiveExtractionReply(journal.ID, "The user shared a sensitive health detail.")}
	extraction := memory.NewExtractionService(repo, fake, "fake-chat", "fake", nil, testLogger())

	if err := extraction.ProcessJournal(ctx, journal.ID); err != nil {
		t.Fatalf("extraction without consent: %v", err)
	}
	if count := countMemories(t, db, user.User.ID); count != 0 {
		t.Fatalf("memories without consent = %d, want 0", count)
	}

	granted := do(t, router, http.MethodPut, "/api/v1/memory-consent", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"granted": true},
	})
	if granted.Status != http.StatusOK {
		t.Fatalf("grant consent = %d: %s", granted.Status, granted.Body)
	}
	if consent := decodeJSON[consentEnvelope](t, granted.Body).Data; !consent.Granted || consent.GrantedAt == nil {
		t.Errorf("granted consent = %+v", consent)
	}

	if err := extraction.ProcessJournal(ctx, journal.ID); err != nil {
		t.Fatalf("extraction with consent: %v", err)
	}
	if count := countMemories(t, db, user.User.ID); count != 1 {
		t.Fatalf("memories with consent = %d, want 1", count)
	}

	list := decodeJSON[sensitiveMemoryListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/memories", requestOptions{token: user.AccessToken}).Body)
	if len(list.Data) != 1 || list.Data[0].Sensitivity != memory.SensitivitySensitive {
		t.Fatalf("sensitive memories = %+v", list.Data)
	}
	memoryID := list.Data[0].ID

	confirmed := do(t, router, http.MethodPost, memoryAPIPath(memoryID)+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "confirm"},
	})
	if confirmed.Status != http.StatusOK {
		t.Fatalf("confirm sensitive memory = %d: %s", confirmed.Status, confirmed.Body)
	}

	service := memory.NewService(repo, testLogger())
	retrieved, err := service.Retrieve(ctx, user.User.ID, "sensitive health detail", 5)
	if err != nil {
		t.Fatalf("retrieve with consent: %v", err)
	}
	if len(retrieved) != 1 || retrieved[0].Sensitivity != memory.SensitivitySensitive {
		t.Errorf("retrieval with consent = %+v", retrieved)
	}

	insightRepo := insight.NewRepository(db.Gorm)
	eligible, err := insightRepo.EligibleMemories(ctx, user.User.ID, 10)
	if err != nil {
		t.Fatalf("eligible memories with consent: %v", err)
	}
	if len(eligible) != 1 {
		t.Errorf("eligible insight memories with consent = %d, want 1", len(eligible))
	}

	revoked := do(t, router, http.MethodPut, "/api/v1/memory-consent", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"granted": false},
	})
	if revoked.Status != http.StatusOK {
		t.Fatalf("revoke consent = %d: %s", revoked.Status, revoked.Body)
	}
	revokedConsent := decodeJSON[consentEnvelope](t, revoked.Body).Data
	if revokedConsent.Granted || revokedConsent.RevokedAt == nil {
		t.Errorf("revoked consent = %+v", revokedConsent)
	}

	retrieved, err = service.Retrieve(ctx, user.User.ID, "sensitive health detail", 5)
	if err != nil {
		t.Fatalf("retrieve after revoke: %v", err)
	}
	if len(retrieved) != 0 {
		t.Errorf("sensitive memory appeared after revocation: %+v", retrieved)
	}

	eligible, err = insightRepo.EligibleMemories(ctx, user.User.ID, 10)
	if err != nil {
		t.Fatalf("eligible memories after revoke: %v", err)
	}
	if len(eligible) != 0 {
		t.Errorf("sensitive memory remained eligible for insights after revocation: %+v", eligible)
	}
}

func countMemories(t *testing.T, db *database.DB, userID string) int {
	t.Helper()

	var count int
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT count(*) FROM personal_memories WHERE user_id = $1 AND deleted_at IS NULL`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	return count
}
