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
	"github.com/ridwanafazn/sela-server/internal/modules/journalanalysis"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type journalAnalysisEvidencePayload struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	SourceDate *string `json:"source_date"`
	Note       string  `json:"note"`
}

type journalAnalysisPayload struct {
	ID                    string                           `json:"id"`
	JournalID             string                           `json:"journal_id"`
	JournalVersion        int                              `json:"journal_version"`
	CurrentJournalVersion int                              `json:"current_journal_version"`
	Stale                 bool                             `json:"stale"`
	EntryDate             string                           `json:"entry_date"`
	Summary               string                           `json:"summary"`
	Observation           string                           `json:"observation"`
	Interpretation        string                           `json:"interpretation"`
	Hypothesis            *string                          `json:"hypothesis"`
	Confidence            float64                          `json:"confidence"`
	ConfidenceLabel       string                           `json:"confidence_label"`
	ReflectionQuestion    string                           `json:"reflection_question"`
	SuggestedDirection    string                           `json:"suggested_direction"`
	Evidence              []journalAnalysisEvidencePayload `json:"evidence"`
	Model                 *string                          `json:"model"`
	PromptVersion         *string                          `json:"prompt_version"`
	CreatedAt             time.Time                        `json:"created_at"`
}

type journalAnalysisEnvelope struct {
	Data  journalAnalysisPayload `json:"data"`
	Error errorPayload           `json:"error"`
}

type journalAnalysisAcceptedPayload struct {
	JobID             string `json:"job_id"`
	Status            string `json:"status"`
	StatusURL         string `json:"status_url"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

type journalAnalysisAcceptedEnvelope struct {
	Data  journalAnalysisAcceptedPayload `json:"data"`
	Error errorPayload                   `json:"error"`
}

type journalAnalysisJobEnvelope struct {
	Data struct {
		JobID      string  `json:"job_id"`
		Status     string  `json:"status"`
		Outcome    *string `json:"outcome"`
		AnalysisID *string `json:"analysis_id"`
		Message    *string `json:"message"`
		Error      *struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
		RetryAfterSeconds int       `json:"retry_after_seconds"`
		CreatedAt         time.Time `json:"created_at"`
		UpdatedAt         time.Time `json:"updated_at"`
	} `json:"data"`
	Error errorPayload `json:"error"`
}

func newJournalAnalysisWorker(db *database.DB, generation *journalanalysis.GenerationService) *job.Worker {
	queue := job.NewQueue(db.Gorm)
	return job.NewWorker(queue, map[string]job.Handler{
		job.TypeJournalAnalysis: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			if claimed.UserID == nil || *claimed.UserID == "" {
				return ai.ErrInvalidRequest
			}
			var payload journalanalysis.JobPayload
			if err := json.Unmarshal(claimed.Payload, &payload); err != nil {
				return err
			}
			result, err := generation.Process(ctx, claimed.ID, *claimed.UserID, payload)
			if err != nil {
				return err
			}
			return queue.SetResult(ctx, claimed.ID, result)
		}),
		job.TypeJournalEmbedding: job.HandlerFunc(func(context.Context, *job.Job) error {
			return nil
		}),
		job.TypeJournalMemoryExtraction: job.HandlerFunc(func(context.Context, *job.Job) error {
			return nil
		}),
	}, "journal-analysis-test-worker", time.Millisecond, testLogger())
}

func journalAnalysisReply() string {
	payload := map[string]any{
		"summary":             "Work pressure and recovery appear in the same entry.",
		"observation":         "The entry links a difficult workday with poor sleep and replaying a conversation.",
		"interpretation":      "Workload may be affecting recovery, but one entry cannot prove a lasting pattern.",
		"hypothesis":          "Protecting a wind-down period could make sleep easier tonight.",
		"confidence":          0.66,
		"reflection_question": "What would make tonight's wind-down easier?",
		"suggested_direction": "improve",
		"evidence_note":       "The entry connects the workday, replaying the conversation, and poor sleep.",
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func postJournalAnalysisAPI(t *testing.T, router *gin.Engine, token, journalID, key string) apiResult {
	t.Helper()

	headers := map[string]string{}
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return do(t, router, http.MethodPost, "/api/v1/journals/"+journalID+"/analysis", requestOptions{
		token:   token,
		headers: headers,
	})
}

const analysisJournalContent = "I had a difficult day at work and stayed late again. I could not sleep because I kept replaying the conversation with my manager. I want to protect my evenings better, but I am not sure where to start."

func TestJournalAnalysisFlow(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)
	stranger := newTestUser(t, router)

	entry := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "A difficult day",
		"content":    analysisJournalContent,
		"entry_date": "2026-11-21",
	})

	key := uuid.NewString()
	accepted := postJournalAnalysisAPI(t, router, user.AccessToken, entry.ID, key)
	if accepted.Status != http.StatusAccepted {
		t.Fatalf("request analysis = %d: %s", accepted.Status, accepted.Body)
	}
	acceptedPayload := decodeJSON[journalAnalysisAcceptedEnvelope](t, accepted.Body).Data
	if acceptedPayload.Status != "queued" || acceptedPayload.JobID == "" {
		t.Fatalf("accepted = %+v", acceptedPayload)
	}
	if acceptedPayload.StatusURL != "/api/v1/journal-analysis-jobs/"+acceptedPayload.JobID {
		t.Errorf("status_url = %q", acceptedPayload.StatusURL)
	}

	replay := postJournalAnalysisAPI(t, router, user.AccessToken, entry.ID, key)
	if replay.Status != http.StatusAccepted {
		t.Fatalf("idempotent replay = %d: %s", replay.Status, replay.Body)
	}
	if got := decodeJSON[journalAnalysisAcceptedEnvelope](t, replay.Body).Data.JobID; got != acceptedPayload.JobID {
		t.Errorf("replay job = %s, want %s", got, acceptedPayload.JobID)
	}
	var jobCount int
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*) FROM scheduled_jobs WHERE user_id = $1 AND job_type = 'journal.analysis'`,
		user.User.ID,
	).Scan(&jobCount); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if jobCount != 1 {
		t.Errorf("analysis jobs = %d, want 1", jobCount)
	}

	queued := do(t, router, http.MethodGet, "/api/v1/journal-analysis-jobs/"+acceptedPayload.JobID, requestOptions{token: user.AccessToken})
	if queued.Status != http.StatusOK {
		t.Fatalf("queued job = %d: %s", queued.Status, queued.Body)
	}
	queuedJob := decodeJSON[journalAnalysisJobEnvelope](t, queued.Body).Data
	if queuedJob.Status != "queued" || queuedJob.Outcome != nil {
		t.Errorf("queued job = %+v", queuedJob)
	}

	fake := &ai.FakeLLM{StructuredReply: journalAnalysisReply()}
	usage := aiusage.NewService(
		db.Gorm,
		aiusage.Prices{InputPerMillion: 2, OutputPerMillion: 8},
		aiusage.Budgets{},
		testLogger(),
	)
	generation := journalanalysis.NewGenerationService(
		journalanalysis.NewRepository(db.Gorm),
		journal.NewRepository(db.Gorm),
		fake,
		"fake-chat",
		"fake",
		usage,
		testLogger(),
	)
	processAllJobs(t, newJournalAnalysisWorker(db, generation))

	completed := decodeJSON[journalAnalysisJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/journal-analysis-jobs/"+acceptedPayload.JobID,
		requestOptions{token: user.AccessToken}).Body).Data
	if completed.Status != "completed" || completed.Outcome == nil || *completed.Outcome != journalanalysis.OutcomeAnalysisCreated {
		t.Fatalf("completed job = %+v", completed)
	}
	if completed.AnalysisID == nil {
		t.Fatal("completed job is missing analysis_id")
	}

	var usageCount, referenceMatches int
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*), count(*) FILTER (WHERE reference_id = $2)
		 FROM ai_usage_events
		 WHERE user_id = $1 AND operation = 'journal.analysis' AND success = true`,
		user.User.ID, *completed.AnalysisID,
	).Scan(&usageCount, &referenceMatches); err != nil {
		t.Fatalf("load analysis usage: %v", err)
	}
	if usageCount != 1 || referenceMatches != 1 {
		t.Errorf("analysis usage = (count %d, reference %d)", usageCount, referenceMatches)
	}

	analysis := decodeJSON[journalAnalysisEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/journals/"+entry.ID+"/analysis",
		requestOptions{token: user.AccessToken}).Body).Data
	if analysis.ID != *completed.AnalysisID || analysis.JournalVersion != 1 || analysis.CurrentJournalVersion != 1 || analysis.Stale {
		t.Errorf("analysis = %+v", analysis)
	}
	if analysis.Summary == "" || analysis.ReflectionQuestion == "" || analysis.ConfidenceLabel != journalanalysis.ConfidenceModerate {
		t.Errorf("analysis content = %+v", analysis)
	}
	if len(analysis.Evidence) != 1 || analysis.Evidence[0].SourceID != entry.ID || analysis.Evidence[0].SourceType != "journal" {
		t.Errorf("analysis evidence = %+v", analysis.Evidence)
	}
	if analysis.Evidence[0].SourceDate == nil || *analysis.Evidence[0].SourceDate != "2026-11-21" {
		t.Errorf("analysis source_date = %+v", analysis.Evidence[0].SourceDate)
	}

	conflict := postJournalAnalysisAPI(t, router, user.AccessToken, entry.ID, uuid.NewString())
	if conflict.Status != http.StatusConflict {
		t.Errorf("duplicate analysis = %d, want 409: %s", conflict.Status, conflict.Body)
	}

	updated := do(t, router, http.MethodPatch, "/api/v1/journals/"+entry.ID, requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "1"},
		body: map[string]any{
			"content": "I had another difficult day at work and stayed late again. I could not sleep because I kept replaying the conversation with my manager. I want to protect my evenings better, and I am considering a fixed shutdown time.",
		},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update journal = %d: %s", updated.Status, updated.Body)
	}
	updatedEntry := decodeJSON[journalEnvelope](t, updated.Body).Data
	if updatedEntry.Version != 2 {
		t.Fatalf("updated journal version = %d, want 2", updatedEntry.Version)
	}

	stale := decodeJSON[journalAnalysisEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/journals/"+entry.ID+"/analysis",
		requestOptions{token: user.AccessToken}).Body).Data
	if !stale.Stale || stale.JournalVersion != 1 || stale.CurrentJournalVersion != 2 {
		t.Errorf("stale analysis = %+v", stale)
	}

	secondAccepted := postJournalAnalysisAPI(t, router, user.AccessToken, entry.ID, uuid.NewString())
	if secondAccepted.Status != http.StatusAccepted {
		t.Fatalf("second analysis request = %d: %s", secondAccepted.Status, secondAccepted.Body)
	}
	processAllJobs(t, newJournalAnalysisWorker(db, generation))
	second := decodeJSON[journalAnalysisEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/journals/"+entry.ID+"/analysis",
		requestOptions{token: user.AccessToken}).Body).Data
	if second.JournalVersion != 2 || second.Stale {
		t.Errorf("second analysis = %+v", second)
	}

	draft := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content": "A draft entry with enough words to be meaningful, but it is not published yet and must not be analyzed by the AI pipeline.",
		"status":  "draft",
	})
	if result := postJournalAnalysisAPI(t, router, user.AccessToken, draft.ID, uuid.NewString()); result.Status != http.StatusConflict {
		t.Errorf("draft analysis = %d, want 409: %s", result.Status, result.Body)
	}

	if result := do(t, router, http.MethodGet, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger analysis = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/journal-analysis-jobs/"+acceptedPayload.JobID, requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger job = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodDelete, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger delete = %d, want 404", result.Status)
	}

	if result := do(t, router, http.MethodDelete, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: user.AccessToken}); result.Status != http.StatusNoContent {
		t.Fatalf("delete analysis = %d: %s", result.Status, result.Body)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: user.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("analysis after delete = %d, want 404", result.Status)
	}
	var remaining int
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*) FROM journal_analyses WHERE user_id = $1 AND journal_id = $2`,
		user.User.ID, entry.ID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count analyses: %v", err)
	}
	if remaining != 0 {
		t.Errorf("analyses after delete = %d, want 0", remaining)
	}

	reRequested := postJournalAnalysisAPI(t, router, user.AccessToken, entry.ID, uuid.NewString())
	if reRequested.Status != http.StatusAccepted {
		t.Fatalf("re-request after delete = %d: %s", reRequested.Status, reRequested.Body)
	}
	processAllJobs(t, newJournalAnalysisWorker(db, generation))
	if result := do(t, router, http.MethodGet, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: user.AccessToken}); result.Status != http.StatusOK {
		t.Errorf("analysis after re-request = %d, want 200: %s", result.Status, result.Body)
	}

	if result := do(t, router, http.MethodDelete, "/api/v1/journals/"+entry.ID, requestOptions{
		token:   user.AccessToken,
		headers: map[string]string{"If-Match": "2"},
	}); result.Status != http.StatusNoContent {
		t.Fatalf("delete journal = %d: %s", result.Status, result.Body)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/journals/"+entry.ID+"/analysis", requestOptions{token: user.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("analysis after journal delete = %d, want 404", result.Status)
	}
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*) FROM journal_analyses WHERE user_id = $1 AND journal_id = $2`,
		user.User.ID, entry.ID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count analyses after journal delete: %v", err)
	}
	if remaining != 0 {
		t.Errorf("analyses after journal delete = %d, want 0", remaining)
	}
}
