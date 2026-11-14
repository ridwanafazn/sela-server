// Package ratelimit implements the DB-backed authentication rate limiter.
// Redis is intentionally not used.
package ratelimit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Decision is the outcome of a rate-limit check.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Limiter stores hashed rate-limit events in PostgreSQL.
type Limiter struct {
	db      *gorm.DB
	secret  []byte
	records atomic.Uint64
}

// New creates a limiter. The secret is used as an HMAC key so raw IP
// addresses and account identifiers are never stored.
func New(db *gorm.DB, secret string) *Limiter {
	return &Limiter{db: db, secret: []byte(secret)}
}

// Check reports whether the subject has reached the limit for the action
// within the window. It does not record an attempt.
func (l *Limiter) Check(ctx context.Context, action, subject string, limit int, window time.Duration) (Decision, error) {
	if limit < 1 {
		return Decision{Allowed: true}, nil
	}

	now := time.Now().UTC()
	cutoff := now.Add(-window)

	var (
		count  int64
		oldest sql.NullTime
	)

	row := l.db.WithContext(ctx).Raw(
		`SELECT count(*), min(created_at)
		 FROM auth_rate_limits
		 WHERE action = ? AND subject_hash = ? AND created_at > ?`,
		action, l.hash(action, subject), cutoff,
	).Row()
	if err := row.Scan(&count, &oldest); err != nil {
		return Decision{}, fmt.Errorf("check rate limit: %w", err)
	}

	if count < int64(limit) {
		return Decision{Allowed: true}, nil
	}

	retryAfter := window
	if oldest.Valid {
		retryAfter = oldest.Time.Add(window).Sub(now)
	}
	if retryAfter < time.Second {
		retryAfter = time.Second
	}

	return Decision{Allowed: false, RetryAfter: retryAfter}, nil
}

// Record stores an attempt for the subject. It also performs opportunistic
// cleanup of expired events so the table does not grow without bound before
// the Phase 3 maintenance job exists.
func (l *Limiter) Record(ctx context.Context, action, subject string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate rate limit id: %w", err)
	}

	if err := l.db.WithContext(ctx).Exec(
		`INSERT INTO auth_rate_limits (id, action, subject_hash, created_at)
		 VALUES (?, ?, ?, ?)`,
		id.String(), action, l.hash(action, subject), time.Now().UTC(),
	).Error; err != nil {
		return fmt.Errorf("record rate limit: %w", err)
	}

	if l.records.Add(1)%100 == 0 {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.Cleanup(cleanupCtx, 48*time.Hour)
	}

	return nil
}

// Cleanup removes events older than the retention period.
func (l *Limiter) Cleanup(ctx context.Context, retention time.Duration) error {
	return l.db.WithContext(ctx).Exec(
		`DELETE FROM auth_rate_limits WHERE created_at < ?`,
		time.Now().UTC().Add(-retention),
	).Error
}

func (l *Limiter) hash(action, subject string) string {
	mac := hmac.New(sha256.New, l.secret)
	_, _ = mac.Write([]byte(action))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(subject))
	return hex.EncodeToString(mac.Sum(nil))
}
