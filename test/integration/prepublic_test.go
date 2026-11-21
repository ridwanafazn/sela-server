//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type outboxMessage struct {
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	SentAt  time.Time `json:"sent_at"`
}

func latestOutboxToken(t *testing.T, dir, subject string) string {
	t.Helper()

	path := filepath.Join(dir, "outbox.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read email outbox: %v", err)
	}
	pattern := regexp.MustCompile(`Token: ([A-Za-z0-9_-]+)`)
	var token string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var message outboxMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("decode email outbox line: %v", err)
		}
		if message.Subject == subject {
			match := pattern.FindStringSubmatch(message.Text)
			if len(match) == 2 {
				token = match[1]
			}
		}
	}
	if token == "" {
		t.Fatalf("no token found for subject %q in %s", subject, path)
	}
	return token
}

func TestEmailVerificationAndPasswordResetFlow(t *testing.T) {
	cfg := testConfig()
	cfg.Email.Provider = "file"
	cfg.Email.OutboxDir = t.TempDir()
	cfg.PrePublic.RequireEmailVerification = true
	router, _ := newAPI(t, cfg)

	email := "verify-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "") + "@example.com"
	registered := register(t, router, email, testPassword, "", "203.0.113.250")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	auth := decodeJSON[authEnvelope](t, registered.Body).Data

	meBefore := decodeJSON[meEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me", requestOptions{
		token: auth.AccessToken,
	}).Body).Data
	if meBefore.User.EmailVerified {
		t.Fatal("new account is already email verified")
	}

	blocked := login(t, router, email, testPassword, "203.0.113.250")
	if blocked.Status != http.StatusForbidden {
		t.Fatalf("login before verification = %d, want 403: %s", blocked.Status, blocked.Body)
	}
	if decodeJSON[errorEnvelope](t, blocked.Body).Error.Code != "EMAIL_NOT_VERIFIED" {
		t.Errorf("login error = %s", blocked.Body)
	}

	verifyToken := latestOutboxToken(t, cfg.Email.OutboxDir, "Verify your SELA email")
	confirmed := do(t, router, http.MethodPost, "/api/v1/auth/email-verification/confirm", requestOptions{
		body: map[string]any{"token": verifyToken},
	})
	if confirmed.Status != http.StatusOK {
		t.Fatalf("confirm verification = %d: %s", confirmed.Status, confirmed.Body)
	}
	if reused := do(t, router, http.MethodPost, "/api/v1/auth/email-verification/confirm", requestOptions{
		body: map[string]any{"token": verifyToken},
	}); reused.Status != http.StatusBadRequest {
		t.Errorf("reused verification token = %d, want 400", reused.Status)
	}

	loggedIn := login(t, router, email, testPassword, "203.0.113.250")
	if loggedIn.Status != http.StatusOK {
		t.Fatalf("login after verification = %d: %s", loggedIn.Status, loggedIn.Body)
	}
	loggedAuth := decodeJSON[authEnvelope](t, loggedIn.Body).Data
	meAfter := decodeJSON[meEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me", requestOptions{
		token: loggedAuth.AccessToken,
	}).Body).Data
	if !meAfter.User.EmailVerified || meAfter.User.EmailVerifiedAt == nil {
		t.Errorf("verified user = %+v", meAfter.User)
	}

	resetRequest := do(t, router, http.MethodPost, "/api/v1/auth/password-reset/request", requestOptions{
		body: map[string]any{"email": email},
		ip:   "203.0.113.250",
	})
	if resetRequest.Status != http.StatusAccepted {
		t.Fatalf("password reset request = %d: %s", resetRequest.Status, resetRequest.Body)
	}
	resetToken := latestOutboxToken(t, cfg.Email.OutboxDir, "Reset your SELA password")
	newPassword := "NewLocalSmoke!2026"
	resetConfirm := do(t, router, http.MethodPost, "/api/v1/auth/password-reset/confirm", requestOptions{
		body: map[string]any{"token": resetToken, "new_password": newPassword},
	})
	if resetConfirm.Status != http.StatusNoContent {
		t.Fatalf("password reset confirm = %d: %s", resetConfirm.Status, resetConfirm.Body)
	}
	if reused := do(t, router, http.MethodPost, "/api/v1/auth/password-reset/confirm", requestOptions{
		body: map[string]any{"token": resetToken, "new_password": newPassword},
	}); reused.Status != http.StatusBadRequest {
		t.Errorf("reused reset token = %d, want 400", reused.Status)
	}

	if oldLogin := login(t, router, email, testPassword, "203.0.113.250"); oldLogin.Status != http.StatusUnauthorized {
		t.Errorf("old password login = %d, want 401", oldLogin.Status)
	}
	if newLogin := login(t, router, email, newPassword, "203.0.113.250"); newLogin.Status != http.StatusOK {
		t.Fatalf("new password login = %d: %s", newLogin.Status, newLogin.Body)
	}
	if oldSession := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{
		token: loggedAuth.AccessToken,
	}); oldSession.Status != http.StatusUnauthorized {
		t.Errorf("old session after reset = %d, want 401", oldSession.Status)
	}
}

type sessionPayload struct {
	ID         string     `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	Current    bool       `json:"current"`
}

type sessionListEnvelope struct {
	Data  []sessionPayload `json:"data"`
	Error errorPayload     `json:"error"`
}

func TestSessionInventoryAndRevocation(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	secondLogin := login(t, router, userEmailFromToken(t, router, user.AccessToken), testPassword, "203.0.113.251")
	if secondLogin.Status != http.StatusOK {
		t.Fatalf("second login = %d: %s", secondLogin.Status, secondLogin.Body)
	}
	second := decodeJSON[authEnvelope](t, secondLogin.Body).Data

	list := decodeJSON[sessionListEnvelope](t, do(t, router, http.MethodGet, "/api/v1/auth/sessions", requestOptions{
		token: second.AccessToken,
	}).Body)
	if len(list.Data) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list.Data))
	}
	currentCount := 0
	var otherID string
	for _, session := range list.Data {
		if session.Current {
			currentCount++
			continue
		}
		otherID = session.ID
	}
	if currentCount != 1 || otherID == "" {
		t.Fatalf("sessions = %+v, want one current and one other", list.Data)
	}

	revoked := do(t, router, http.MethodDelete, "/api/v1/auth/sessions/"+otherID, requestOptions{
		token: second.AccessToken,
	})
	if revoked.Status != http.StatusNoContent {
		t.Fatalf("revoke session = %d: %s", revoked.Status, revoked.Body)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: user.AccessToken}); result.Status != http.StatusUnauthorized {
		t.Errorf("revoked first session = %d, want 401", result.Status)
	}

	if result := do(t, router, http.MethodDelete, "/api/v1/auth/sessions", requestOptions{
		token: second.AccessToken,
	}); result.Status != http.StatusNoContent {
		t.Fatalf("revoke other sessions = %d: %s", result.Status, result.Body)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: second.AccessToken}); result.Status != http.StatusOK {
		t.Errorf("current session after revoke-others = %d, want 200", result.Status)
	}
}

func userEmailFromToken(t *testing.T, router *gin.Engine, token string) string {
	t.Helper()

	me := decodeJSON[meEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me", requestOptions{
		token: token,
	}).Body).Data
	return me.User.Email
}

type prepublicConsentEnvelope struct {
	Data struct {
		AdultConfirmed         bool       `json:"adult_confirmed"`
		AdultConfirmedAt       *time.Time `json:"adult_confirmed_at"`
		TermsAccepted          bool       `json:"terms_accepted"`
		TermsVersion           *string    `json:"terms_version"`
		PrivacyAccepted        bool       `json:"privacy_accepted"`
		PrivacyVersion         *string    `json:"privacy_version"`
		SensitiveMemoryGranted bool       `json:"sensitive_memory_granted"`
	} `json:"data"`
	Error errorPayload `json:"error"`
}

type entitlementEnvelope struct {
	Data struct {
		Plan   string `json:"plan"`
		Status string `json:"status"`
	} `json:"data"`
	Error errorPayload `json:"error"`
}

func TestConsentsEntitlementAndExport(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	initial := decodeJSON[prepublicConsentEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me/consents", requestOptions{
		token: user.AccessToken,
	}).Body).Data
	if initial.AdultConfirmed || initial.TermsAccepted || initial.PrivacyAccepted {
		t.Fatalf("initial consents = %+v, want empty", initial)
	}

	updated := do(t, router, http.MethodPut, "/api/v1/me/consents", requestOptions{
		token: user.AccessToken,
		body: map[string]any{
			"adult_confirmed": true,
			"terms_version":   "2026-11-21",
			"privacy_version": "2026-11-21",
		},
	})
	if updated.Status != http.StatusOK {
		t.Fatalf("update consents = %d: %s", updated.Status, updated.Body)
	}
	summary := decodeJSON[prepublicConsentEnvelope](t, updated.Body).Data
	if !summary.AdultConfirmed || !summary.TermsAccepted || !summary.PrivacyAccepted {
		t.Errorf("updated consents = %+v", summary)
	}

	stale := do(t, router, http.MethodPut, "/api/v1/me/consents", requestOptions{
		token: user.AccessToken,
		body:  map[string]any{"terms_version": "old-version"},
	})
	if stale.Status != http.StatusBadRequest {
		t.Errorf("stale terms = %d, want 400", stale.Status)
	}

	entitlement := decodeJSON[entitlementEnvelope](t, do(t, router, http.MethodGet, "/api/v1/me/entitlements", requestOptions{
		token: user.AccessToken,
	}).Body).Data
	if entitlement.Plan != "free" || entitlement.Status != "active" {
		t.Errorf("entitlement = %+v", entitlement)
	}

	journal := mustCreateJournal(t, router, user.AccessToken, map[string]any{
		"content":    "Export this journal content.",
		"entry_date": "2026-11-21",
	})
	export := do(t, router, http.MethodGet, "/api/v1/me/export", requestOptions{
		token: user.AccessToken,
	})
	if export.Status != http.StatusOK {
		t.Fatalf("export = %d: %s", export.Status, export.Body)
	}
	if !strings.Contains(export.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("Content-Disposition = %q", export.Header.Get("Content-Disposition"))
	}
	if strings.Contains(string(export.Body), "password_hash") || strings.Contains(string(export.Body), "refresh_token_hash") {
		t.Error("export contains secret metadata")
	}

	var exported map[string]any
	if err := json.Unmarshal(export.Body, &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported["schema_version"] != "prepublic/1" {
		t.Errorf("schema_version = %v", exported["schema_version"])
	}
	account, ok := exported["account"].(map[string]any)
	if !ok || account["email"] != userEmailFromToken(t, router, user.AccessToken) {
		t.Errorf("export account = %+v", exported["account"])
	}
	journals, ok := exported["journals"].([]any)
	if !ok || len(journals) != 1 {
		t.Fatalf("export journals = %+v", exported["journals"])
	}
	firstJournal := journals[0].(map[string]any)
	if firstJournal["id"] != journal.ID || firstJournal["content"] != "Export this journal content." {
		t.Errorf("export journal = %+v", firstJournal)
	}
	if _, ok := exported["auth_sessions"].([]any); !ok {
		t.Errorf("export auth_sessions = %+v", exported["auth_sessions"])
	}
}
