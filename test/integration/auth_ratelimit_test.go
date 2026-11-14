//go:build integration

package integration_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestLoginRateLimitPerIP(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.LoginPerMinuteIP = 2
	cfg.RateLimit.LoginPerHourAccount = 1000
	router, _ := newAPI(t, cfg)

	const email = "limit-ip@example.com"
	if result := register(t, router, email, testPassword, "", "203.0.113.80"); result.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", result.Status, result.Body)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if result := login(t, router, email, "wrong password", "203.0.113.80"); result.Status != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401: %s", attempt+1, result.Status, result.Body)
		}
	}

	limited := login(t, router, email, "wrong password", "203.0.113.80")
	if limited.Status != http.StatusTooManyRequests {
		t.Fatalf("third attempt = %d, want 429: %s", limited.Status, limited.Body)
	}

	envelope := decodeJSON[errorEnvelope](t, limited.Body)
	if envelope.Error.Code != "RATE_LIMITED" {
		t.Errorf("error code = %q, want RATE_LIMITED", envelope.Error.Code)
	}

	retryAfter, err := strconv.Atoi(limited.Header.Get("Retry-After"))
	if err != nil || retryAfter < 1 {
		t.Errorf("Retry-After = %q, want a positive integer", limited.Header.Get("Retry-After"))
	}

	otherIP := login(t, router, email, "wrong password", "203.0.113.81")
	if otherIP.Status != http.StatusUnauthorized {
		t.Fatalf("attempt from another IP = %d, want 401: %s", otherIP.Status, otherIP.Body)
	}
}

func TestLoginRateLimitPerAccount(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.LoginPerMinuteIP = 1000
	cfg.RateLimit.LoginPerHourAccount = 2
	router, _ := newAPI(t, cfg)

	const email = "limit-account@example.com"
	if result := register(t, router, email, testPassword, "", "203.0.113.82"); result.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", result.Status, result.Body)
	}

	ips := []string{"203.0.113.82", "203.0.113.83", "203.0.113.84"}
	for attempt := 0; attempt < 2; attempt++ {
		if result := login(t, router, email, "wrong password", ips[attempt]); result.Status != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401: %s", attempt+1, result.Status, result.Body)
		}
	}

	limited := login(t, router, email, "wrong password", ips[2])
	if limited.Status != http.StatusTooManyRequests {
		t.Fatalf("third attempt from a new IP = %d, want 429: %s", limited.Status, limited.Body)
	}
}

func TestRegisterRateLimitPerIP(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.RegisterPerHourIP = 1
	router, _ := newAPI(t, cfg)

	const ip = "203.0.113.85"
	if result := register(t, router, "register-one@example.com", testPassword, "", ip); result.Status != http.StatusCreated {
		t.Fatalf("first register = %d: %s", result.Status, result.Body)
	}

	limited := register(t, router, "register-two@example.com", testPassword, "", ip)
	if limited.Status != http.StatusTooManyRequests {
		t.Fatalf("second register from the same IP = %d, want 429: %s", limited.Status, limited.Body)
	}

	if result := register(t, router, "register-three@example.com", testPassword, "", "203.0.113.86"); result.Status != http.StatusCreated {
		t.Fatalf("register from another IP = %d, want 201: %s", result.Status, result.Body)
	}
}

func TestRefreshRateLimitPerIP(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.RefreshPerMinuteIP = 1
	router, _ := newAPI(t, cfg)

	const ip = "203.0.113.87"
	registered := register(t, router, "refresh-limit@example.com", testPassword, "", ip)
	if registered.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", registered.Status, registered.Body)
	}
	authBody := decodeJSON[authEnvelope](t, registered.Body)

	first := refresh(t, router, authBody.Data.RefreshToken, ip)
	if first.Status != http.StatusOK {
		t.Fatalf("first refresh = %d, want 200: %s", first.Status, first.Body)
	}
	rotated := decodeJSON[authEnvelope](t, first.Body)

	limited := refresh(t, router, rotated.Data.RefreshToken, ip)
	if limited.Status != http.StatusTooManyRequests {
		t.Fatalf("second refresh from the same IP = %d, want 429: %s", limited.Status, limited.Body)
	}

	otherIP := refresh(t, router, rotated.Data.RefreshToken, "203.0.113.88")
	if otherIP.Status != http.StatusOK {
		t.Fatalf("refresh from another IP = %d, want 200: %s", otherIP.Status, otherIP.Body)
	}
}

func TestRateLimitSubjectsAreHashed(t *testing.T) {
	cfg := testConfig()
	router, db := newAPI(t, cfg)

	const email = "hashed-subject@example.com"
	if result := register(t, router, email, testPassword, "", "203.0.113.89"); result.Status != http.StatusCreated {
		t.Fatalf("register = %d: %s", result.Status, result.Body)
	}
	if result := login(t, router, email, "wrong password", "203.0.113.89"); result.Status != http.StatusUnauthorized {
		t.Fatalf("login = %d, want 401: %s", result.Status, result.Body)
	}

	rows, err := db.SQL.QueryContext(context.Background(),
		`SELECT action, subject_hash FROM auth_rate_limits WHERE action LIKE 'auth.login.%'`,
	)
	if err != nil {
		t.Fatalf("query rate limits: %v", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	count := 0
	for rows.Next() {
		var action, subject string
		if err := rows.Scan(&action, &subject); err != nil {
			t.Fatalf("scan rate limit: %v", err)
		}
		count++

		if len(subject) != 64 {
			t.Errorf("%s hash length = %d, want 64", action, len(subject))
		}
		if _, err := hex.DecodeString(subject); err != nil {
			t.Errorf("%s hash is not hex: %v", action, err)
		}
		if strings.Contains(subject, "hashed-subject") || strings.Contains(subject, "203.0.113.89") {
			t.Errorf("%s stored a raw subject", action)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rate limits: %v", err)
	}
	if count != 2 {
		t.Errorf("login rate limit rows = %d, want 2 (IP and account)", count)
	}
}
