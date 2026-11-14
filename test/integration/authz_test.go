//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/auth"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

func sessionIDForUser(t *testing.T, db *database.DB, userID string) string {
	t.Helper()

	var sessionID string
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT id FROM auth_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&sessionID); err != nil {
		t.Fatalf("load session: %v", err)
	}
	return sessionID
}

func TestExpiredSessionIsRejected(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)

	if _, err := db.SQL.ExecContext(context.Background(),
		`UPDATE auth_sessions SET expires_at = now() - interval '1 hour' WHERE user_id = $1`,
		user.User.ID,
	); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	me := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: user.AccessToken})
	if me.Status != http.StatusUnauthorized {
		t.Fatalf("expired session /me = %d, want 401: %s", me.Status, me.Body)
	}

	rotated := refresh(t, router, user.RefreshToken, "203.0.113.220")
	if rotated.Status != http.StatusUnauthorized {
		t.Fatalf("expired refresh = %d, want 401: %s", rotated.Status, rotated.Body)
	}

	var revoked *time.Time
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT revoked_at FROM auth_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		user.User.ID,
	).Scan(&revoked); err != nil {
		t.Fatalf("load revoked_at: %v", err)
	}
	if revoked == nil {
		t.Error("expired session was not revoked")
	}
}

func TestDeletionPendingUserIsRejected(t *testing.T) {
	router, db := newAPI(t, testConfig())

	email := "deletion-pending@example.com"
	registered := register(t, router, email, testPassword, "", "203.0.113.221")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	user := decodeJSON[authEnvelope](t, registered.Body).Data

	if _, err := db.SQL.ExecContext(context.Background(),
		`UPDATE users SET status = 'deletion_pending', deletion_requested_at = now() WHERE email = $1`,
		email,
	); err != nil {
		t.Fatalf("set deletion_pending: %v", err)
	}

	if me := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: user.AccessToken}); me.Status != http.StatusUnauthorized {
		t.Errorf("deletion_pending /me = %d, want 401: %s", me.Status, me.Body)
	}
	if loginResult := login(t, router, email, testPassword, "203.0.113.221"); loginResult.Status != http.StatusUnauthorized {
		t.Errorf("deletion_pending login = %d, want 401: %s", loginResult.Status, loginResult.Body)
	}
	if rotated := refresh(t, router, user.RefreshToken, "203.0.113.221"); rotated.Status != http.StatusUnauthorized {
		t.Errorf("deletion_pending refresh = %d, want 401: %s", rotated.Status, rotated.Body)
	}

	var activeSessions int
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT count(*) FROM auth_sessions WHERE user_id = $1 AND revoked_at IS NULL`,
		user.User.ID,
	).Scan(&activeSessions); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	if activeSessions != 0 {
		t.Errorf("active sessions = %d, want 0 after refresh rejection", activeSessions)
	}
}

func TestDeletedUserRefreshIsRejected(t *testing.T) {
	router, db := newAPI(t, testConfig())

	email := "deleted-refresh@example.com"
	registered := register(t, router, email, testPassword, "", "203.0.113.222")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	user := decodeJSON[authEnvelope](t, registered.Body).Data

	if _, err := db.SQL.ExecContext(context.Background(),
		`UPDATE users SET status = 'deleted', deleted_at = now() WHERE email = $1`,
		email,
	); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	rotated := refresh(t, router, user.RefreshToken, "203.0.113.222")
	if rotated.Status != http.StatusUnauthorized {
		t.Fatalf("deleted user refresh = %d, want 401: %s", rotated.Status, rotated.Body)
	}
}

func TestTokenForUnknownSessionIsRejected(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	forged, _, err := auth.NewTokenManager(testJWTSecret, "sela-api", "sela-mobile", 15*time.Minute).
		Issue(user.User.ID, uuid.NewString())
	if err != nil {
		t.Fatalf("craft token: %v", err)
	}

	result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: forged})
	if result.Status != http.StatusUnauthorized {
		t.Fatalf("token for unknown session = %d, want 401: %s", result.Status, result.Body)
	}
}

func TestProfileUpdatesAreIsolated(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	first := newTestUser(t, router)
	second := newTestUser(t, router)

	update := do(t, router, http.MethodPatch, "/api/v1/me", requestOptions{
		token: first.AccessToken,
		body: map[string]any{
			"display_name": "First user",
			"language":     "en-US",
		},
	})
	if update.Status != http.StatusOK {
		t.Fatalf("update first profile = %d: %s", update.Status, update.Body)
	}

	secondMe := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: second.AccessToken})
	if secondMe.Status != http.StatusOK {
		t.Fatalf("get second profile = %d: %s", secondMe.Status, secondMe.Body)
	}

	profile := decodeJSON[meEnvelope](t, secondMe.Body).Data.Profile
	if profile.DisplayName != nil {
		t.Errorf("second display_name = %v, want null", profile.DisplayName)
	}
	if profile.Language != "id" {
		t.Errorf("second language = %q, want id", profile.Language)
	}
}

func TestSessionIDCannotBeReusedAcrossUsers(t *testing.T) {
	router, db := newAPI(t, testConfig())
	first := newTestUser(t, router)
	second := newTestUser(t, router)

	firstSessionID := sessionIDForUser(t, db, first.User.ID)

	forged, _, err := auth.NewTokenManager(testJWTSecret, "sela-api", "sela-mobile", 15*time.Minute).
		Issue(second.User.ID, firstSessionID)
	if err != nil {
		t.Fatalf("craft token: %v", err)
	}

	result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: forged})
	if result.Status != http.StatusUnauthorized {
		t.Fatalf("cross-user session = %d, want 401: %s", result.Status, result.Body)
	}
}
