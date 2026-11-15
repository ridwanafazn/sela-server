//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ridwanafazn/sela-server/internal/modules/auth"
)

func TestAuthenticationFlow(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	const email = "Person@Example.com"

	registerResult := register(t, router, email, testPassword, "Ridwan", "203.0.113.10")
	if registerResult.Status != http.StatusCreated {
		t.Fatalf("register status = %d, want 201: %s", registerResult.Status, registerResult.Body)
	}

	registered := decodeJSON[authEnvelope](t, registerResult.Body)
	if registered.Data.User.Email != "person@example.com" {
		t.Errorf("registered email = %q, want normalized lowercase", registered.Data.User.Email)
	}
	if registered.Data.TokenType != "Bearer" || registered.Data.ExpiresIn != 900 {
		t.Errorf("token metadata = (%q, %d), want (Bearer, 900)", registered.Data.TokenType, registered.Data.ExpiresIn)
	}
	if registered.Data.AccessToken == "" || registered.Data.RefreshToken == "" {
		t.Fatal("register response is missing tokens")
	}

	meResult := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: registered.Data.AccessToken})
	if meResult.Status != http.StatusOK {
		t.Fatalf("GET /me status = %d, want 200: %s", meResult.Status, meResult.Body)
	}

	me := decodeJSON[meEnvelope](t, meResult.Body)
	if me.Data.User.Email != "person@example.com" {
		t.Errorf("me email = %q, want person@example.com", me.Data.User.Email)
	}
	if me.Data.Profile.DisplayName == nil || *me.Data.Profile.DisplayName != "Ridwan" {
		t.Errorf("display_name = %v, want Ridwan", me.Data.Profile.DisplayName)
	}
	if me.Data.Profile.Language != "id" || me.Data.Profile.Timezone != "Asia/Jakarta" || me.Data.Profile.CommunicationStyle != "adaptive" {
		t.Errorf("profile defaults = %+v", me.Data.Profile)
	}

	logoutResult := do(t, router, http.MethodPost, "/api/v1/auth/logout", requestOptions{token: registered.Data.AccessToken})
	if logoutResult.Status != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204: %s", logoutResult.Status, logoutResult.Body)
	}

	revokedResult := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: registered.Data.AccessToken})
	if revokedResult.Status != http.StatusUnauthorized {
		t.Fatalf("GET /me after logout = %d, want 401", revokedResult.Status)
	}
	if revoked := decodeJSON[errorEnvelope](t, revokedResult.Body); revoked.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error code = %q, want UNAUTHENTICATED", revoked.Error.Code)
	}

	loginResult := login(t, router, email, testPassword, "203.0.113.10")
	if loginResult.Status != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", loginResult.Status, loginResult.Body)
	}

	loggedIn := decodeJSON[authEnvelope](t, loginResult.Body)
	firstRefresh := loggedIn.Data.RefreshToken

	rotated := refresh(t, router, firstRefresh, "203.0.113.10")
	if rotated.Status != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200: %s", rotated.Status, rotated.Body)
	}

	rotatedBody := decodeJSON[authEnvelope](t, rotated.Body)
	if rotatedBody.Data.RefreshToken == firstRefresh {
		t.Error("refresh token was not rotated")
	}

	reuse := refresh(t, router, firstRefresh, "203.0.113.10")
	if reuse.Status != http.StatusUnauthorized {
		t.Fatalf("reusing a rotated token = %d, want 401: %s", reuse.Status, reuse.Body)
	}

	familyRevoked := refresh(t, router, rotatedBody.Data.RefreshToken, "203.0.113.10")
	if familyRevoked.Status != http.StatusUnauthorized {
		t.Fatalf("refresh after reuse detection = %d, want 401 (family must be revoked)", familyRevoked.Status)
	}

	accessAfterReuse := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: rotatedBody.Data.AccessToken})
	if accessAfterReuse.Status != http.StatusUnauthorized {
		t.Fatalf("access token after family revocation = %d, want 401", accessAfterReuse.Status)
	}
}

func TestDuplicateRegistrationIsCaseInsensitive(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	if result := register(t, router, "duplicate@example.com", testPassword, "", "203.0.113.20"); result.Status != http.StatusCreated {
		t.Fatalf("first register = %d, want 201: %s", result.Status, result.Body)
	}

	result := register(t, router, "DUPLICATE@example.com", testPassword, "", "203.0.113.20")
	if result.Status != http.StatusConflict {
		t.Fatalf("duplicate register = %d, want 409: %s", result.Status, result.Body)
	}

	envelope := decodeJSON[errorEnvelope](t, result.Body)
	if envelope.Error.Code != "CONFLICT" {
		t.Errorf("error code = %q, want CONFLICT", envelope.Error.Code)
	}
}

func TestLoginRejectsWrongPasswordAndDeletedAccount(t *testing.T) {
	router, db := newAPI(t, testConfig())

	const email = "deleted@example.com"
	registered := register(t, router, email, testPassword, "", "203.0.113.30")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d, want 201: %s", registered.Status, registered.Body)
	}
	registeredBody := decodeJSON[authEnvelope](t, registered.Body)

	wrong := login(t, router, email, "wrong password", "203.0.113.30")
	if wrong.Status != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401: %s", wrong.Status, wrong.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, wrong.Body); envelope.Error.Code != "INVALID_CREDENTIALS" {
		t.Errorf("error code = %q, want INVALID_CREDENTIALS", envelope.Error.Code)
	}

	if _, err := db.SQL.ExecContext(context.Background(),
		`UPDATE users SET status = 'deleted', deleted_at = now() WHERE email = $1`, email,
	); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}

	blocked := login(t, router, email, testPassword, "203.0.113.30")
	if blocked.Status != http.StatusUnauthorized {
		t.Fatalf("deleted account login = %d, want 401: %s", blocked.Status, blocked.Body)
	}

	sessionBlocked := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: registeredBody.Data.AccessToken})
	if sessionBlocked.Status != http.StatusUnauthorized {
		t.Fatalf("deleted account session = %d, want 401", sessionBlocked.Status)
	}
}

func TestUpdateMe(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	registered := register(t, router, "profile-update@example.com", testPassword, "Ridwan", "203.0.113.40")
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d, want 201: %s", registered.Status, registered.Body)
	}
	authBody := decodeJSON[authEnvelope](t, registered.Body)

	update := do(t, router, http.MethodPatch, "/api/v1/me", requestOptions{
		token: authBody.Data.AccessToken,
		body: map[string]any{
			"display_name":        nil,
			"language":            "en-US",
			"timezone":            "Asia/Makassar",
			"communication_style": "direct",
		},
	})
	if update.Status != http.StatusOK {
		t.Fatalf("PATCH /me = %d, want 200: %s", update.Status, update.Body)
	}

	updated := decodeJSON[meEnvelope](t, update.Body)
	if updated.Data.Profile.DisplayName != nil {
		t.Errorf("display_name = %v, want null", updated.Data.Profile.DisplayName)
	}
	if updated.Data.Profile.Language != "en-US" ||
		updated.Data.Profile.Timezone != "Asia/Makassar" ||
		updated.Data.Profile.CommunicationStyle != "direct" {
		t.Errorf("profile = %+v, want updated values", updated.Data.Profile)
	}

	invalidStyle := do(t, router, http.MethodPatch, "/api/v1/me", requestOptions{
		token: authBody.Data.AccessToken,
		body:  map[string]any{"communication_style": "harsh"},
	})
	if invalidStyle.Status != http.StatusBadRequest {
		t.Fatalf("invalid style = %d, want 400: %s", invalidStyle.Status, invalidStyle.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, invalidStyle.Body); envelope.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", envelope.Error.Code)
	}

	invalidTimezone := do(t, router, http.MethodPatch, "/api/v1/me", requestOptions{
		token: authBody.Data.AccessToken,
		body:  map[string]any{"timezone": "Mars/Olympus"},
	})
	if invalidTimezone.Status != http.StatusBadRequest {
		t.Fatalf("invalid timezone = %d, want 400: %s", invalidTimezone.Status, invalidTimezone.Body)
	}

	noop := do(t, router, http.MethodPatch, "/api/v1/me", requestOptions{
		token: authBody.Data.AccessToken,
		body:  map[string]any{},
	})
	if noop.Status != http.StatusOK {
		t.Fatalf("empty patch = %d, want 200: %s", noop.Status, noop.Body)
	}

	current := decodeJSON[meEnvelope](t, noop.Body)
	if current.Data.Profile.Language != "en-US" || current.Data.Profile.CommunicationStyle != "direct" {
		t.Errorf("empty patch changed the profile: %+v", current.Data.Profile)
	}
}

func TestProtectedEndpointsRequireAuthentication(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		token  string
	}{
		{name: "me without token", method: http.MethodGet, path: "/api/v1/me"},
		{name: "me with garbage token", method: http.MethodGet, path: "/api/v1/me", token: "garbage"},
		{name: "logout without token", method: http.MethodPost, path: "/api/v1/auth/logout"},
		{name: "patch me without token", method: http.MethodPatch, path: "/api/v1/me", body: map[string]any{"language": "id"}},
		{name: "delete me without token", method: http.MethodDelete, path: "/api/v1/me", body: map[string]any{"password": "x", "confirmation": "DELETE"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := do(t, router, test.method, test.path, requestOptions{body: test.body, token: test.token})
			if result.Status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", result.Status, result.Body)
			}
			if envelope := decodeJSON[errorEnvelope](t, result.Body); envelope.Error.Code != "UNAUTHENTICATED" {
				t.Errorf("error code = %q, want UNAUTHENTICATED", envelope.Error.Code)
			}
		})
	}
}

func TestAccessTokenCannotBeUsedForAnotherUser(t *testing.T) {
	router, db := newAPI(t, testConfig())
	ctx := context.Background()

	first := register(t, router, "first@example.com", testPassword, "", "203.0.113.50")
	if first.Status != http.StatusCreated {
		t.Fatalf("register first = %d: %s", first.Status, first.Body)
	}
	second := register(t, router, "second@example.com", testPassword, "", "203.0.113.50")
	if second.Status != http.StatusCreated {
		t.Fatalf("register second = %d: %s", second.Status, second.Body)
	}

	var firstSessionID string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT s.id FROM auth_sessions s JOIN users u ON u.id = s.user_id WHERE u.email = $1 ORDER BY s.created_at DESC LIMIT 1`,
		"first@example.com",
	).Scan(&firstSessionID); err != nil {
		t.Fatalf("load first session: %v", err)
	}

	var secondUserID string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = $1`, "second@example.com",
	).Scan(&secondUserID); err != nil {
		t.Fatalf("load second user: %v", err)
	}

	forged, _, err := auth.NewTokenManager(testJWTSecret, "sela-api", "sela-mobile", 15*time.Minute).
		Issue(secondUserID, firstSessionID)
	if err != nil {
		t.Fatalf("craft token: %v", err)
	}

	result := do(t, router, http.MethodGet, "/api/v1/me", requestOptions{token: forged})
	if result.Status != http.StatusUnauthorized {
		t.Fatalf("forged subject = %d, want 401: %s", result.Status, result.Body)
	}
}

func TestRegisterValidation(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	invalidEmail := register(t, router, "not-an-email", testPassword, "", "203.0.113.60")
	if invalidEmail.Status != http.StatusBadRequest {
		t.Fatalf("invalid email = %d, want 400: %s", invalidEmail.Status, invalidEmail.Body)
	}
	envelope := decodeJSON[errorEnvelope](t, invalidEmail.Body)
	if envelope.Error.Code != "VALIDATION_ERROR" || len(envelope.Error.Details) == 0 {
		t.Errorf("error = %+v, want VALIDATION_ERROR with details", envelope.Error)
	}

	shortPassword := register(t, router, "short@example.com", "short", "", "203.0.113.60")
	if shortPassword.Status != http.StatusBadRequest {
		t.Fatalf("short password = %d, want 400: %s", shortPassword.Status, shortPassword.Body)
	}

	noContentType := do(t, router, http.MethodPost, "/api/v1/auth/register", requestOptions{
		raw:    `{"email":"noctype@example.com","password":"` + testPassword + `"}`,
		ip:     "203.0.113.60",
		noJSON: true,
	})
	if noContentType.Status != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type = %d, want 415: %s", noContentType.Status, noContentType.Body)
	}
}

func TestOversizedRequestIsRejected(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.MaxBodyBytes = 64
	router, _ := newAPI(t, cfg)

	result := register(t, router, "oversized@example.com", testPassword, "", "203.0.113.61")
	if result.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized register = %d, want 413: %s", result.Status, result.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, result.Body); envelope.Error.Code != "PAYLOAD_TOO_LARGE" {
		t.Errorf("error code = %q, want PAYLOAD_TOO_LARGE", envelope.Error.Code)
	}
}

func TestRefreshWithUnknownToken(t *testing.T) {
	router, _ := newAPI(t, testConfig())

	result := refresh(t, router, "unknown-refresh-token", "203.0.113.70")
	if result.Status != http.StatusUnauthorized {
		t.Fatalf("unknown refresh token = %d, want 401: %s", result.Status, result.Body)
	}
}
