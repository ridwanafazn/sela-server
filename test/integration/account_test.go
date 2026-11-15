//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/account"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

type deletionPayload struct {
	Status           string    `json:"status"`
	PurgeScheduledAt time.Time `json:"purge_scheduled_at"`
	Message          string    `json:"message"`
}

type deletionEnvelope struct {
	Data  deletionPayload `json:"data"`
	Error errorPayload    `json:"error"`
}

func newAccountWorker(db *database.DB) *job.Worker {
	queue := job.NewQueue(db.Gorm)
	repository := account.NewRepository(db.Gorm)
	noop := job.HandlerFunc(func(context.Context, *job.Job) error { return nil })

	return job.NewWorker(queue, map[string]job.Handler{
		job.TypeAccountDelete: job.HandlerFunc(func(ctx context.Context, claimed *job.Job) error {
			if claimed.UserID == nil {
				return nil
			}
			_, err := repository.Purge(ctx, *claimed.UserID)
			return err
		}),
		job.TypeJournalEmbedding:        noop,
		job.TypeJournalMemoryExtraction: noop,
		job.TypeMessageMemoryExtraction: noop,
		job.TypeInsightGenerate:         noop,
	}, "account-test-worker", time.Millisecond, testLogger())
}

func TestAccountDeletionRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.AccountDeletePerHour = 1
	router, _ := newAPI(t, cfg)

	registered := register(t, router, "delete-limit-"+uuid.NewString()+"@example.com", testPassword, "", "203.0.113.251")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	auth := decodeJSON[authEnvelope](t, registered.Body).Data

	first := do(t, router, http.MethodDelete, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
		body:  map[string]any{"password": "wrong password", "confirmation": "DELETE"},
	})
	if first.Status != http.StatusUnauthorized {
		t.Fatalf("first attempt = %d, want 401", first.Status)
	}

	second := do(t, router, http.MethodDelete, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
		body:  map[string]any{"password": testPassword, "confirmation": "DELETE"},
	})
	if second.Status != http.StatusTooManyRequests {
		t.Fatalf("second attempt = %d, want 429: %s", second.Status, second.Body)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}

	if result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: auth.AccessToken}); result.Status != http.StatusOK {
		t.Errorf("account was changed after a rate-limited attempt: %d", result.Status)
	}
}

func TestAccountDeletionDeactivatesAndPurges(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()

	email := "delete-account-" + uuid.NewString() + "@example.com"
	registered := register(t, router, email, testPassword, "", "203.0.113.250")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	auth := decodeJSON[authEnvelope](t, registered.Body).Data
	userID := auth.User.ID

	journal := mustCreateJournal(t, router, auth.AccessToken, map[string]any{
		"title":      "Account data",
		"content":    "This entry must be purged with the account.",
		"entry_date": "2026-11-14",
	})
	conversation := mustCreateConversation(t, router, auth.AccessToken, "Account deletion")
	sent := sendMessageAPI(t, router, auth.AccessToken, conversation.ID, uuid.NewString(), "A message that must be purged.")
	if sent.Status != http.StatusOK {
		t.Fatalf("send message = %d: %s", sent.Status, sent.Body)
	}

	memoryRepo := memory.NewRepository(db.Gorm)
	category := "test"
	confidence := 0.8
	seedMemory := memory.Memory{
		ID:          uuid.NewString(),
		UserID:      userID,
		Content:     "A memory that must be purged.",
		Category:    &category,
		Status:      memory.StatusActive,
		Confidence:  &confidence,
		SourceCount: 1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := memoryRepo.Create(ctx, &seedMemory, journalSourceRef(journal.ID), nil); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	wrongConfirmation := do(t, router, http.MethodDelete, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
		body:  map[string]any{"password": testPassword, "confirmation": "delete"},
	})
	if wrongConfirmation.Status != http.StatusBadRequest {
		t.Errorf("wrong confirmation = %d, want 400", wrongConfirmation.Status)
	}

	wrongPassword := do(t, router, http.MethodDelete, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
		body:  map[string]any{"password": "wrong password", "confirmation": "DELETE"},
	})
	if wrongPassword.Status != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", wrongPassword.Status)
	}

	deleted := do(t, router, http.MethodDelete, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
		body:  map[string]any{"password": testPassword, "confirmation": "DELETE"},
	})
	if deleted.Status != http.StatusAccepted {
		t.Fatalf("delete account = %d: %s", deleted.Status, deleted.Body)
	}
	payload := decodeJSON[deletionEnvelope](t, deleted.Body).Data
	if payload.Status != "pending" || payload.Message == "" || payload.PurgeScheduledAt.IsZero() {
		t.Errorf("deletion response = %+v", payload)
	}

	if result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: auth.AccessToken}); result.Status != http.StatusUnauthorized {
		t.Errorf("get me after deletion = %d, want 401", result.Status)
	}
	if result := login(t, router, email, testPassword, "203.0.113.250"); result.Status != http.StatusUnauthorized {
		t.Errorf("login after deletion = %d, want 401", result.Status)
	}
	if result := refresh(t, router, auth.RefreshToken, "203.0.113.250"); result.Status != http.StatusUnauthorized {
		t.Errorf("refresh after deletion = %d, want 401", result.Status)
	}

	var status string
	var deletionRequestedAt *time.Time
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT status, deletion_requested_at FROM users WHERE id = $1`, userID,
	).Scan(&status, &deletionRequestedAt); err != nil {
		t.Fatalf("load user status: %v", err)
	}
	if status != "deletion_pending" || deletionRequestedAt == nil {
		t.Errorf("user status = (%s, %v), want deletion_pending with timestamp", status, deletionRequestedAt)
	}

	var revokedSessions, pendingJobs int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NOT NULL`, userID,
	).Scan(&revokedSessions); err != nil {
		t.Fatalf("count revoked sessions: %v", err)
	}
	if revokedSessions < 1 {
		t.Errorf("revoked sessions = %d, want at least 1", revokedSessions)
	}
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_jobs WHERE user_id = $1 AND job_type = $2 AND status = 'pending'`,
		userID, job.TypeAccountDelete,
	).Scan(&pendingJobs); err != nil {
		t.Fatalf("count purge jobs: %v", err)
	}
	if pendingJobs != 1 {
		t.Errorf("pending purge jobs = %d, want 1", pendingJobs)
	}

	processAllJobs(t, newAccountWorker(db))

	var users, journals, conversations, messages, memories, sessions, jobs, idempotency, usage int
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM journals WHERE user_id = $1`, userID).Scan(&journals); err != nil {
		t.Fatalf("count journals: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM conversations WHERE user_id = $1`, userID).Scan(&conversations); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE user_id = $1`, userID).Scan(&messages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM personal_memories WHERE user_id = $1`, userID).Scan(&memories); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM auth_sessions WHERE user_id = $1`, userID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM scheduled_jobs WHERE user_id = $1`, userID).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM idempotency_keys WHERE user_id = $1`, userID).Scan(&idempotency); err != nil {
		t.Fatalf("count idempotency keys: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM ai_usage_events WHERE user_id = $1`, userID).Scan(&usage); err != nil {
		t.Fatalf("count ai usage events: %v", err)
	}
	if users != 0 || journals != 0 || conversations != 0 || messages != 0 || memories != 0 || sessions != 0 || jobs != 0 || idempotency != 0 || usage != 0 {
		t.Errorf("purged data = users:%d journals:%d conversations:%d messages:%d memories:%d sessions:%d jobs:%d idempotency:%d usage:%d",
			users, journals, conversations, messages, memories, sessions, jobs, idempotency, usage)
	}

	reRegistered := register(t, router, email, testPassword, "", "203.0.113.250")
	if reRegistered.Status != http.StatusCreated {
		t.Errorf("re-register after purge = %d: %s", reRegistered.Status, reRegistered.Body)
	}
}
