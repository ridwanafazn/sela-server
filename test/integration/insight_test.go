//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type insightEvidencePayload struct {
	ID            string     `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	Note          string     `json:"note"`
	SourceDate    *time.Time `json:"source_date"`
	SourceDeleted bool       `json:"source_deleted"`
}

type insightFeedbackPayload struct {
	Status    string     `json:"status"`
	Note      *string    `json:"note"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type insightPayload struct {
	ID                 string                   `json:"id"`
	Title              string                   `json:"title"`
	Summary            string                   `json:"summary"`
	Observation        string                   `json:"observation"`
	Interpretation     string                   `json:"interpretation"`
	Hypothesis         *string                  `json:"hypothesis"`
	Confidence         float64                  `json:"confidence"`
	ConfidenceLabel    string                   `json:"confidence_label"`
	ReflectionQuestion string                   `json:"reflection_question"`
	SuggestedDirection string                   `json:"suggested_direction"`
	PeriodStart        *string                  `json:"period_start"`
	PeriodEnd          *string                  `json:"period_end"`
	GenerationType     string                   `json:"generation_type"`
	Evidence           []insightEvidencePayload `json:"evidence"`
	Feedback           insightFeedbackPayload   `json:"feedback"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

type insightEnvelope struct {
	Data  insightPayload `json:"data"`
	Error errorPayload   `json:"error"`
}

type insightListItemPayload struct {
	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
	Summary            string                 `json:"summary"`
	Confidence         float64                `json:"confidence"`
	ConfidenceLabel    string                 `json:"confidence_label"`
	SuggestedDirection string                 `json:"suggested_direction"`
	PeriodStart        *string                `json:"period_start"`
	PeriodEnd          *string                `json:"period_end"`
	GenerationType     string                 `json:"generation_type"`
	EvidenceCount      int                    `json:"evidence_count"`
	Feedback           insightFeedbackPayload `json:"feedback"`
	CreatedAt          time.Time              `json:"created_at"`
}

type insightListEnvelope struct {
	Data       []insightListItemPayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Error errorPayload `json:"error"`
}

type insightJobAcceptedPayload struct {
	JobID             string `json:"job_id"`
	Status            string `json:"status"`
	StatusURL         string `json:"status_url"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

type insightJobEnvelope struct {
	Data struct {
		JobID     string  `json:"job_id"`
		Status    string  `json:"status"`
		Outcome   *string `json:"outcome"`
		InsightID *string `json:"insight_id"`
		Message   *string `json:"message"`
		Error     *struct {
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

type insightAcceptedEnvelope struct {
	Data  insightJobAcceptedPayload `json:"data"`
	Error errorPayload              `json:"error"`
}

func newInsightWorker(db *database.DB, generation *insight.GenerationService) *job.Worker {
	queue := job.NewQueue(db.Gorm)
	return job.NewWorker(queue, map[string]job.Handler{
		job.TypeInsightGenerate: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			if claimed.UserID == nil || *claimed.UserID == "" {
				return ai.ErrInvalidRequest
			}
			var payload insight.JobPayload
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
		job.TypeMessageMemoryExtraction: job.HandlerFunc(func(context.Context, *job.Job) error {
			return nil
		}),
	}, "insight-test-worker", time.Millisecond, testLogger())
}

func postInsightAPI(t *testing.T, router *gin.Engine, token, key string, body any) apiResult {
	t.Helper()

	headers := map[string]string{}
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return do(t, router, http.MethodPost, "/api/v1/insights", requestOptions{
		token:   token,
		ip:      "203.0.113.240",
		headers: headers,
		body:    body,
	})
}

func insightReply(sourceIDs []string, confidence float64) string {
	evidence := make([]map[string]string, 0, len(sourceIDs))
	for index, sourceID := range sourceIDs {
		evidence = append(evidence, map[string]string{
			"source_type": "journal",
			"source_id":   sourceID,
			"note":        fmt.Sprintf("Entry %d supports the observed pattern.", index+1),
		})
	}
	payload := map[string]any{
		"title":               "A recurring work pattern",
		"summary":             "Work appears to affect rest and evening boundaries.",
		"observation":         "Both entries connect work with reduced recovery time.",
		"interpretation":      "Work boundaries may be difficult to maintain right now.",
		"hypothesis":          "Protecting one evening per week could improve recovery.",
		"confidence":          confidence,
		"reflection_question": "What would one protected evening look like this week?",
		"suggested_direction": "improve",
		"evidence":            evidence,
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func TestInsightGenerationAndFeedbackFlow(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)
	stranger := newTestUser(t, router)

	first := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Late night",
		"content":    "I could not sleep well because of work.",
		"entry_date": "2026-11-10",
	})
	second := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"title":      "Evening boundary",
		"content":    "I want to protect my evenings from work.",
		"entry_date": "2026-11-12",
	})

	key := uuid.NewString()
	body := map[string]any{
		"period":     "custom",
		"start_date": "2026-11-01",
		"end_date":   "2026-11-15",
	}
	accepted := postInsightAPI(t, router, user.AccessToken, key, body)
	if accepted.Status != http.StatusAccepted {
		t.Fatalf("request insight = %d: %s", accepted.Status, accepted.Body)
	}
	acceptedPayload := decodeJSON[insightAcceptedEnvelope](t, accepted.Body).Data
	if acceptedPayload.Status != "queued" || acceptedPayload.JobID == "" {
		t.Fatalf("accepted = %+v", acceptedPayload)
	}
	if acceptedPayload.StatusURL != "/api/v1/insight-jobs/"+acceptedPayload.JobID {
		t.Errorf("status_url = %q", acceptedPayload.StatusURL)
	}

	queued := do(t, router, http.MethodGet, "/api/v1/insight-jobs/"+acceptedPayload.JobID, requestOptions{token: user.AccessToken})
	if queued.Status != http.StatusOK {
		t.Fatalf("queued job = %d: %s", queued.Status, queued.Body)
	}
	queuedJob := decodeJSON[insightJobEnvelope](t, queued.Body).Data
	if queuedJob.Status != "queued" || queuedJob.Outcome != nil {
		t.Errorf("queued job = %+v", queuedJob)
	}
	if queued.Header.Get("Retry-After") == "" {
		t.Error("queued job is missing Retry-After")
	}

	fake := &ai.FakeLLM{StructuredReply: insightReply([]string{first.ID, second.ID}, 0.72)}
	generation := insight.NewGenerationService(insight.NewRepository(db.Gorm), fake, "fake-chat", testLogger())
	processAllJobs(t, newInsightWorker(db, generation))

	completed := decodeJSON[insightJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insight-jobs/"+acceptedPayload.JobID, requestOptions{token: user.AccessToken}).Body).Data
	if completed.Status != "completed" || completed.Outcome == nil || *completed.Outcome != "insight_created" {
		t.Fatalf("completed job = %+v", completed)
	}
	if completed.InsightID == nil {
		t.Fatal("completed job is missing insight_id")
	}

	detail := decodeJSON[insightEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insights/"+*completed.InsightID, requestOptions{token: user.AccessToken}).Body).Data
	if detail.Title == "" || len(detail.Evidence) != 2 {
		t.Fatalf("insight = %+v", detail)
	}
	if detail.Feedback.Status != insight.FeedbackNone {
		t.Errorf("feedback status = %s, want none", detail.Feedback.Status)
	}
	if detail.ConfidenceLabel != insight.ConfidenceModerate {
		t.Errorf("confidence label = %s, want moderate", detail.ConfidenceLabel)
	}
	if detail.PeriodStart == nil || *detail.PeriodStart != "2026-11-01" {
		t.Errorf("period_start = %v", detail.PeriodStart)
	}
	if detail.GenerationType != insight.GenerationOnDemand {
		t.Errorf("generation_type = %s, want on_demand", detail.GenerationType)
	}

	if result := do(t, router, http.MethodGet, "/api/v1/insights/"+*completed.InsightID, requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger get insight = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/insight-jobs/"+acceptedPayload.JobID, requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger job = %d, want 404", result.Status)
	}

	list := decodeJSON[insightListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insights", requestOptions{token: user.AccessToken}).Body)
	if len(list.Data) != 1 || list.Data[0].EvidenceCount != 2 {
		t.Fatalf("insight list = %+v", list.Data)
	}

	weekly := decodeJSON[insightListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insights?period=weekly", requestOptions{token: user.AccessToken}).Body)
	if len(weekly.Data) != 0 {
		t.Errorf("weekly insights = %d, want 0 for an on-demand insight", len(weekly.Data))
	}

	confirmed := do(t, router, http.MethodPost, "/api/v1/insights/"+*completed.InsightID+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "confirm"},
	})
	if confirmed.Status != http.StatusOK {
		t.Fatalf("confirm feedback = %d: %s", confirmed.Status, confirmed.Body)
	}
	if got := decodeJSON[insightEnvelope](t, confirmed.Body).Data.Feedback.Status; got != insight.FeedbackConfirmed {
		t.Errorf("feedback status = %s, want confirmed", got)
	}

	corrected := do(t, router, http.MethodPost, "/api/v1/insights/"+*completed.InsightID+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "correct", "note": "The sleep link is the strongest part."},
	})
	if corrected.Status != http.StatusOK {
		t.Fatalf("correct feedback = %d: %s", corrected.Status, corrected.Body)
	}
	correctedInsight := decodeJSON[insightEnvelope](t, corrected.Body).Data
	if correctedInsight.Feedback.Status != insight.FeedbackCorrected || correctedInsight.Feedback.Note == nil {
		t.Errorf("corrected feedback = %+v", correctedInsight.Feedback)
	}

	missingNote := do(t, router, http.MethodPost, "/api/v1/insights/"+*completed.InsightID+"/feedback", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"action": "correct"},
	})
	if missingNote.Status != http.StatusBadRequest {
		t.Errorf("correct without note = %d, want 400", missingNote.Status)
	}

	replay := postInsightAPI(t, router, user.AccessToken, key, body)
	if replay.Status != http.StatusAccepted {
		t.Fatalf("idempotent replay = %d: %s", replay.Status, replay.Body)
	}
	if got := decodeJSON[insightAcceptedEnvelope](t, replay.Body).Data.JobID; got != acceptedPayload.JobID {
		t.Errorf("replay job_id = %s, want %s", got, acceptedPayload.JobID)
	}

	reused := postInsightAPI(t, router, user.AccessToken, key, map[string]any{
		"period":     "custom",
		"start_date": "2026-11-02",
		"end_date":   "2026-11-15",
	})
	if reused.Status != http.StatusConflict {
		t.Errorf("reused key = %d, want 409", reused.Status)
	}

	if deleted := do(t, router, http.MethodDelete, "/api/v1/insights/"+*completed.InsightID, requestOptions{token: user.AccessToken}); deleted.Status != http.StatusNoContent {
		t.Fatalf("delete insight = %d: %s", deleted.Status, deleted.Body)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/insights/"+*completed.InsightID, requestOptions{token: user.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", result.Status)
	}
	afterDelete := decodeJSON[insightListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insights", requestOptions{token: user.AccessToken}).Body)
	if len(afterDelete.Data) != 0 {
		t.Errorf("insights after delete = %d, want 0", len(afterDelete.Data))
	}
}

func TestInsightInsufficientEvidence(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)

	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Only one eligible entry.",
		"entry_date": "2026-11-10",
	})

	accepted := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"period":     "custom",
		"start_date": "2026-11-01",
		"end_date":   "2026-11-15",
	})
	if accepted.Status != http.StatusAccepted {
		t.Fatalf("request insight = %d: %s", accepted.Status, accepted.Body)
	}
	jobID := decodeJSON[insightAcceptedEnvelope](t, accepted.Body).Data.JobID

	fake := &ai.FakeLLM{StructuredReply: insightReply([]string{uuid.NewString(), uuid.NewString()}, 0.8)}
	generation := insight.NewGenerationService(insight.NewRepository(db.Gorm), fake, "fake-chat", testLogger())
	processAllJobs(t, newInsightWorker(db, generation))

	jobView := decodeJSON[insightJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insight-jobs/"+jobID, requestOptions{token: user.AccessToken}).Body).Data
	if jobView.Status != "completed" || jobView.Outcome == nil || *jobView.Outcome != "insufficient_evidence" {
		t.Fatalf("job = %+v", jobView)
	}
	if jobView.InsightID != nil {
		t.Errorf("insufficient evidence returned insight_id %v", *jobView.InsightID)
	}
	if jobView.Message == nil || *jobView.Message == "" {
		t.Error("insufficient evidence is missing a message")
	}

	list := decodeJSON[insightListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insights", requestOptions{token: user.AccessToken}).Body)
	if len(list.Data) != 0 {
		t.Errorf("insights = %d, want 0", len(list.Data))
	}
}

func TestInsightLowConfidenceIsInsufficient(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)

	first := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "First entry about work and sleep.",
		"entry_date": "2026-11-10",
	})
	second := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Second entry about work and evenings.",
		"entry_date": "2026-11-11",
	})

	accepted := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"period":     "custom",
		"start_date": "2026-11-01",
		"end_date":   "2026-11-15",
	})
	jobID := decodeJSON[insightAcceptedEnvelope](t, accepted.Body).Data.JobID

	fake := &ai.FakeLLM{StructuredReply: insightReply([]string{first.ID, second.ID}, 0.2)}
	generation := insight.NewGenerationService(insight.NewRepository(db.Gorm), fake, "fake-chat", testLogger())
	processAllJobs(t, newInsightWorker(db, generation))

	jobView := decodeJSON[insightJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insight-jobs/"+jobID, requestOptions{token: user.AccessToken}).Body).Data
	if jobView.Status != "completed" || jobView.Outcome == nil || *jobView.Outcome != "insufficient_evidence" {
		t.Errorf("low confidence job = %+v", jobView)
	}
}

func TestInsightInventedEvidenceFailsJob(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)

	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "First entry about work.",
		"entry_date": "2026-11-10",
	})
	mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Second entry about rest.",
		"entry_date": "2026-11-11",
	})

	accepted := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{
		"period":     "custom",
		"start_date": "2026-11-01",
		"end_date":   "2026-11-15",
	})
	jobID := decodeJSON[insightAcceptedEnvelope](t, accepted.Body).Data.JobID

	fake := &ai.FakeLLM{StructuredReply: insightReply([]string{uuid.NewString(), uuid.NewString()}, 0.8)}
	generation := insight.NewGenerationService(insight.NewRepository(db.Gorm), fake, "fake-chat", testLogger())
	processAllJobs(t, newInsightWorker(db, generation))

	jobView := decodeJSON[insightJobEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/insight-jobs/"+jobID, requestOptions{token: user.AccessToken}).Body).Data
	if jobView.Status != "failed" || jobView.Error == nil {
		t.Fatalf("invented evidence job = %+v", jobView)
	}
	if jobView.Error.Code != "INSIGHT_GENERATION_FAILED" || jobView.Error.Retryable {
		t.Errorf("job error = %+v", jobView.Error)
	}
}

func TestInsightRateLimitAndValidation(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.InsightPerHour = 1
	cfg.RateLimit.InsightPerDay = 10
	router, _ := newAPI(t, cfg)
	user := newTestUser(t, router)

	body := map[string]any{"period": "weekly"}
	first := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), body)
	if first.Status != http.StatusAccepted {
		t.Fatalf("first request = %d: %s", first.Status, first.Body)
	}

	second := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), body)
	if second.Status != http.StatusTooManyRequests {
		t.Errorf("second request = %d, want 429", second.Status)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}

	if replay := postInsightAPI(t, router, user.AccessToken, uuid.NewString(), map[string]any{"period": "custom"}); replay.Status != http.StatusBadRequest {
		t.Errorf("custom without dates = %d, want 400", replay.Status)
	}
	if missing := postInsightAPI(t, router, user.AccessToken, "", body); missing.Status != http.StatusBadRequest {
		t.Errorf("missing idempotency key = %d, want 400", missing.Status)
	}
}
