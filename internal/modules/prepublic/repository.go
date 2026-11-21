package prepublic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repository persists account gates, entitlements, and export data.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the pre-public repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetConsents returns the profile gates and sensitive memory consent.
func (r *Repository) GetConsents(ctx context.Context, userID string) (*ConsentSummary, error) {
	var summary ConsentSummary
	var (
		adultAt  *time.Time
		termsAt  *time.Time
		termsVer *string
		privAt   *time.Time
		privVer  *string
	)
	err := r.db.WithContext(ctx).Raw(
		`SELECT adult_confirmed_at, terms_accepted_at, terms_version,
		        privacy_accepted_at, privacy_version
		 FROM profiles WHERE user_id = ?`,
		userID,
	).Row().Scan(&adultAt, &termsAt, &termsVer, &privAt, &privVer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load consents: %w", err)
	}

	var (
		granted   bool
		updatedAt *time.Time
	)
	err = r.db.WithContext(ctx).Raw(
		`SELECT granted, updated_at
		 FROM memory_consents
		 WHERE user_id = ? AND scope = 'sensitive_memory'`,
		userID,
	).Row().Scan(&granted, &updatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load sensitive memory consent: %w", err)
	}

	summary.AdultConfirmed = adultAt != nil
	summary.AdultConfirmedAt = adultAt
	summary.TermsAccepted = termsAt != nil
	summary.TermsAcceptedAt = termsAt
	summary.TermsVersion = termsVer
	summary.PrivacyAccepted = privAt != nil
	summary.PrivacyAcceptedAt = privAt
	summary.PrivacyVersion = privVer
	summary.SensitiveMemoryGranted = granted
	summary.SensitiveMemoryUpdatedAt = updatedAt
	return &summary, nil
}

// UpdateConsents records adult confirmation and legal acceptance.
func (r *Repository) UpdateConsents(
	ctx context.Context,
	userID string,
	adult bool,
	termsVersion, privacyVersion *string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE profiles
		 SET adult_confirmed_at = CASE
		       WHEN ? THEN COALESCE(adult_confirmed_at, ?)
		       ELSE adult_confirmed_at
		     END,
		     terms_accepted_at = CASE WHEN ?::text IS NOT NULL THEN ? ELSE terms_accepted_at END,
		     terms_version = COALESCE(?::text, terms_version),
		     privacy_accepted_at = CASE WHEN ?::text IS NOT NULL THEN ? ELSE privacy_accepted_at END,
		     privacy_version = COALESCE(?::text, privacy_version),
		     updated_at = ?
		 WHERE user_id = ?`,
		adult, now,
		termsVersion, now,
		termsVersion,
		privacyVersion, now,
		privacyVersion,
		now, userID,
	).Error
}

// GetEntitlement returns the user's plan, creating the free entitlement on
// first access.
func (r *Repository) GetEntitlement(ctx context.Context, userID string) (*Entitlement, error) {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Exec(
		`INSERT INTO user_entitlements (user_id, plan, status, created_at, updated_at)
		 VALUES (?, 'free', 'active', ?, ?)
		 ON CONFLICT (user_id) DO NOTHING`,
		userID, now, now,
	).Error; err != nil {
		return nil, fmt.Errorf("create entitlement: %w", err)
	}

	var entitlement Entitlement
	if err := r.db.WithContext(ctx).Raw(
		`SELECT plan, status, current_period_end, created_at, updated_at
		 FROM user_entitlements WHERE user_id = ?`,
		userID,
	).Scan(&entitlement).Error; err != nil {
		return nil, fmt.Errorf("load entitlement: %w", err)
	}
	return &entitlement, nil
}

// SetEntitlement upserts a plan state for manual support operations.
func (r *Repository) SetEntitlement(
	ctx context.Context,
	userID, plan, status string,
	currentPeriodEnd *time.Time,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO user_entitlements
		 (user_id, plan, status, current_period_end, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		   plan = EXCLUDED.plan,
		   status = EXCLUDED.status,
		   current_period_end = EXCLUDED.current_period_end,
		   updated_at = EXCLUDED.updated_at`,
		userID, plan, status, currentPeriodEnd, now, now,
	).Error
}

// Export returns a machine-readable copy of the user's data without secrets.
func (r *Repository) Export(ctx context.Context, userID string) (map[string]any, error) {
	result := map[string]any{
		"schema_version": "prepublic/1",
		"exported_at":    time.Now().UTC(),
	}

	account := map[string]any{}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT u.id, u.email, u.email_verified_at, u.created_at, u.updated_at,
		        p.display_name, p.language, p.timezone, p.communication_style,
		        p.adult_confirmed_at, p.terms_accepted_at, p.terms_version,
		        p.privacy_accepted_at, p.privacy_version
		 FROM users u JOIN profiles p ON p.user_id = u.id
		 WHERE u.id = ?`,
		userID,
	).Scan(&account).Error; err != nil {
		return nil, fmt.Errorf("export account: %w", err)
	}
	result["account"] = account

	sections := []struct {
		name  string
		query string
	}{
		{
			name: "journals",
			query: `SELECT id, title, content, entry_date, status, version,
			               created_at, updated_at, deleted_at
			        FROM journals WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "conversations",
			query: `SELECT id, title, created_at, updated_at, deleted_at
			        FROM conversations WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "messages",
			query: `SELECT id, conversation_id, role, content, metadata, created_at
			        FROM messages WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "memories",
			query: `SELECT id, content, category, sensitivity, status, confidence,
			               source_count, supersedes_memory_id, last_confirmed_at,
			               created_at, updated_at, deleted_at
			        FROM personal_memories WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "memory_sources",
			query: `SELECT ms.id, ms.memory_id, ms.journal_id, ms.message_id,
			               ms.source_excerpt, ms.created_at
			        FROM memory_sources ms
			        JOIN personal_memories pm ON pm.id = ms.memory_id
			        WHERE pm.user_id = ? ORDER BY ms.created_at, ms.id`,
		},
		{
			name: "memory_consents",
			query: `SELECT scope, granted, granted_at, revoked_at, created_at, updated_at
			        FROM memory_consents WHERE user_id = ? ORDER BY scope`,
		},
		{
			name: "memory_conflicts",
			query: `SELECT mc.id, mc.memory_id, mc.conflicting_memory_id, mc.created_at
			        FROM memory_conflicts mc
			        JOIN personal_memories pm ON pm.id = mc.memory_id
			        WHERE pm.user_id = ? ORDER BY mc.created_at, mc.id`,
		},
		{
			name: "insights",
			query: `SELECT id, title, summary, observation, interpretation, hypothesis,
			               confidence, confidence_label, reflection_question,
			               suggested_direction, period_start, period_end, generation_type,
			               model, prompt_version, created_at, updated_at, deleted_at
			        FROM insights WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "insight_evidence",
			query: `SELECT e.id, e.insight_id, e.journal_id, e.message_id, e.memory_id,
			               e.note, e.source_deleted_at, e.created_at
			        FROM insight_evidence e JOIN insights i ON i.id = e.insight_id
			        WHERE i.user_id = ? ORDER BY e.created_at, e.id`,
		},
		{
			name: "insight_feedback",
			query: `SELECT id, insight_id, action, note, created_at
			        FROM insight_feedback WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "insight_schedules",
			query: `SELECT cadence, timezone, enabled, next_run_at, last_run_at, created_at, updated_at
			        FROM insight_schedules WHERE user_id = ?`,
		},
		{
			name: "journal_analyses",
			query: `SELECT id, journal_id, journal_version, summary, observation,
			               interpretation, hypothesis, confidence, confidence_label,
			               reflection_question, suggested_direction, evidence_note,
			               model, prompt_version, created_at
			        FROM journal_analyses WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "ai_usage_events",
			query: `SELECT operation, provider, model, input_tokens, output_tokens,
			               estimated_cost_usd, latency_ms, success, error_category, created_at
			        FROM ai_usage_events WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "auth_sessions",
			query: `SELECT id, created_at, last_used_at, expires_at, revoked_at
			        FROM auth_sessions WHERE user_id = ? ORDER BY created_at, id`,
		},
		{
			name: "entitlement",
			query: `SELECT plan, status, current_period_end, created_at, updated_at
			        FROM user_entitlements WHERE user_id = ?`,
		},
	}

	for _, section := range sections {
		rows := []map[string]any{}
		if err := r.db.WithContext(ctx).Raw(section.query, userID).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("export %s: %w", section.name, err)
		}
		result[section.name] = rows
	}

	return result, nil
}
