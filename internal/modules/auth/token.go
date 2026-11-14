package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrInvalidToken is returned for any access token that fails validation.
var ErrInvalidToken = errors.New("invalid access token")

// Claims is the JWT payload. The session identifier lets the middleware
// check revocation in the database on every request.
type Claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

// TokenManager issues and validates HS256 access tokens.
type TokenManager struct {
	secret    []byte
	issuer    string
	audience  string
	accessTTL time.Duration
}

// NewTokenManager creates a token manager.
func NewTokenManager(secret, issuer, audience string, accessTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret:    []byte(secret),
		issuer:    issuer,
		audience:  audience,
		accessTTL: accessTTL,
	}
}

// AccessTTL returns the access-token lifetime.
func (m *TokenManager) AccessTTL() time.Duration {
	return m.accessTTL
}

// Issue signs an access token for the user and session.
func (m *TokenManager) Issue(userID, sessionID string) (string, int, error) {
	now := time.Now().UTC()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	})

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}

	return signed, int(m.accessTTL.Seconds()), nil
}

// Parse validates an access token and returns its claims.
func (m *TokenManager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}

	parsed, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil || !parsed.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if claims.Subject == "" || claims.SessionID == "" {
		return nil, fmt.Errorf("%w: missing subject or session", ErrInvalidToken)
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, fmt.Errorf("%w: invalid subject", ErrInvalidToken)
	}
	if _, err := uuid.Parse(claims.SessionID); err != nil {
		return nil, fmt.Errorf("%w: invalid session", ErrInvalidToken)
	}

	return claims, nil
}

// NewRefreshToken returns a new opaque refresh token and its storage hash.
func NewRefreshToken() (raw string, hash string, err error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buffer)
	return raw, HashRefreshToken(raw), nil
}

// HashRefreshToken hashes an opaque refresh token for storage. The token has
// full entropy, so SHA-256 is sufficient and constant-time comparison is not
// required for database lookups.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
