package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	testSecret   = "test-only-secret-0123456789abcdefghij"
	testIssuer   = "sela-api"
	testAudience = "sela-mobile"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	manager := NewTokenManager(testSecret, testIssuer, testAudience, 15*time.Minute)
	userID := uuid.NewString()
	sessionID := uuid.NewString()

	raw, expiresIn, err := manager.Issue(userID, sessionID)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if expiresIn != 900 {
		t.Errorf("expiresIn = %d, want 900", expiresIn)
	}

	claims, err := manager.Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("subject = %q, want %q", claims.Subject, userID)
	}
	if claims.SessionID != sessionID {
		t.Errorf("session = %q, want %q", claims.SessionID, sessionID)
	}
	if claims.Issuer != testIssuer {
		t.Errorf("issuer = %q, want %q", claims.Issuer, testIssuer)
	}
}

func TestAccessTokenRejectsInvalidTokens(t *testing.T) {
	issuer := NewTokenManager(testSecret, testIssuer, testAudience, 15*time.Minute)
	userID := uuid.NewString()
	sessionID := uuid.NewString()

	valid, _, err := issuer.Issue(userID, sessionID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tests := []struct {
		name    string
		manager *TokenManager
		token   string
	}{
		{
			name:    "wrong secret",
			manager: NewTokenManager("another-secret-0123456789abcdefghij", testIssuer, testAudience, time.Minute),
			token:   valid,
		},
		{
			name:    "wrong issuer",
			manager: NewTokenManager(testSecret, "other-issuer", testAudience, time.Minute),
			token:   valid,
		},
		{
			name:    "wrong audience",
			manager: NewTokenManager(testSecret, testIssuer, "other-audience", time.Minute),
			token:   valid,
		},
		{
			name:    "expired",
			manager: NewTokenManager(testSecret, testIssuer, testAudience, -time.Hour),
			token:   mustIssue(t, NewTokenManager(testSecret, testIssuer, testAudience, -time.Hour), userID, sessionID),
		},
		{
			name:    "empty subject",
			manager: issuer,
			token:   mustIssue(t, issuer, "", sessionID),
		},
		{
			name:    "empty session",
			manager: issuer,
			token:   mustIssue(t, issuer, userID, ""),
		},
		{
			name:    "garbage",
			manager: issuer,
			token:   "not-a-token",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.manager.Parse(test.token); err == nil {
				t.Error("Parse accepted an invalid token")
			}
		})
	}
}

func TestAccessTokenRejectsWrongAlgorithm(t *testing.T) {
	manager := NewTokenManager(testSecret, testIssuer, testAudience, time.Minute)
	claims := Claims{
		SessionID: uuid.NewString(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   uuid.NewString(),
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	raw, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign HS384 token: %v", err)
	}

	if _, err := manager.Parse(raw); err == nil {
		t.Error("Parse accepted a token signed with HS384")
	}
}

func TestRefreshTokens(t *testing.T) {
	first, firstHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	second, secondHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}

	if first == "" || first == second {
		t.Error("refresh tokens must be unique and non-empty")
	}
	if firstHash != HashRefreshToken(first) {
		t.Error("stored hash does not match the token")
	}
	if firstHash == secondHash {
		t.Error("distinct refresh tokens produced the same hash")
	}
	if len(firstHash) != 64 {
		t.Errorf("hash length = %d, want 64 hex characters", len(firstHash))
	}
}

func mustIssue(t *testing.T, manager *TokenManager, userID, sessionID string) string {
	t.Helper()

	raw, _, err := manager.Issue(userID, sessionID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return raw
}
