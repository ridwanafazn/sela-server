package privacy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repository persists privacy preferences and reads consent state.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the privacy repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Ensure inserts the default row when it does not exist.
func (r *Repository) Ensure(ctx context.Context, userID string, now time.Time) error {
	if err := r.db.WithContext(ctx).Exec(
		`INSERT INTO privacy_preferences
		 (user_id, ai_context_mode, memory_extraction_enabled, created_at, updated_at)
		 VALUES (?, ?, true, ?, ?)
		 ON CONFLICT (user_id) DO NOTHING`,
		userID, ContextJournalsAndMemories, now, now,
	).Error; err != nil {
		return fmt.Errorf("ensure privacy preferences: %w", err)
	}
	return nil
}

// Get returns the persisted preferences after ensuring defaults exist.
func (r *Repository) Get(ctx context.Context, userID string, now time.Time) (*Preferences, error) {
	if err := r.Ensure(ctx, userID, now); err != nil {
		return nil, err
	}
	return r.current(ctx, userID)
}

// Current returns the effective preferences without writing a default row.
func (r *Repository) Current(ctx context.Context, userID string) (*Preferences, error) {
	return r.current(ctx, userID)
}

func (r *Repository) current(ctx context.Context, userID string) (*Preferences, error) {
	var preferences Preferences
	err := r.db.WithContext(ctx).Raw(
		`SELECT COALESCE(
		          (SELECT ai_context_mode FROM privacy_preferences WHERE user_id = ?),
		          ?
		        ) AS ai_context_mode,
		        COALESCE(
		          (SELECT memory_extraction_enabled FROM privacy_preferences WHERE user_id = ?),
		          true
		        ) AS memory_extraction_enabled,
		        COALESCE(
		          (SELECT updated_at FROM privacy_preferences WHERE user_id = ?),
		          now()
		        ) AS updated_at`,
		userID, ContextJournalsAndMemories, userID, userID,
	).Scan(&preferences).Error
	if err != nil {
		return nil, fmt.Errorf("load privacy preferences: %w", err)
	}
	return &preferences, nil
}

// Update applies a partial update and returns the new state.
func (r *Repository) Update(
	ctx context.Context,
	userID string,
	mode *string,
	memoryExtraction *bool,
	now time.Time,
) (*Preferences, error) {
	if err := r.Ensure(ctx, userID, now); err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Exec(
		`UPDATE privacy_preferences
		 SET ai_context_mode = COALESCE(?::text, ai_context_mode),
		     memory_extraction_enabled = COALESCE(?::boolean, memory_extraction_enabled),
		     updated_at = ?
		 WHERE user_id = ?`,
		mode, memoryExtraction, now, userID,
	).Error; err != nil {
		return nil, fmt.Errorf("update privacy preferences: %w", err)
	}
	return r.current(ctx, userID)
}

// ConsentState returns legal and sensitive-memory consent fields.
type ConsentState struct {
	SensitiveMemoryGranted bool
	AdultConfirmed         bool
	TermsVersion           *string
	PrivacyVersion         *string
}

// Consents loads the user-facing consent summary.
func (r *Repository) Consents(ctx context.Context, userID string) (*ConsentState, error) {
	var state ConsentState
	var adultAt *time.Time
	err := r.db.WithContext(ctx).Raw(
		`SELECT p.adult_confirmed_at, p.terms_version, p.privacy_version
		 FROM profiles p WHERE p.user_id = ?`,
		userID,
	).Row().Scan(&adultAt, &state.TermsVersion, &state.PrivacyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load privacy consents: %w", err)
	}
	state.AdultConfirmed = adultAt != nil

	var granted bool
	err = r.db.WithContext(ctx).Raw(
		`SELECT granted FROM memory_consents
		 WHERE user_id = ? AND scope = 'sensitive_memory'`,
		userID,
	).Row().Scan(&granted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load sensitive memory consent: %w", err)
	}
	state.SensitiveMemoryGranted = granted
	return &state, nil
}
