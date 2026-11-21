package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists users, profiles created during registration, and
// refresh sessions.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the auth repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// FindUserByEmail returns the user with the given normalized email.
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUserByID returns the user with the given ID.
func (r *Repository) FindUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUserByIDTx returns the user with the given ID inside a transaction.
func (r *Repository) FindUserByIDTx(tx *gorm.DB, id string) (*User, error) {
	var user User
	if err := tx.Where("id = ?", id).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser inserts a user inside the supplied transaction.
func (r *Repository) CreateUser(tx *gorm.DB, user *User) error {
	return tx.Create(user).Error
}

// CreateProfile inserts the initial profile for a newly registered user.
func (r *Repository) CreateProfile(tx *gorm.DB, id, userID string, displayName *string) error {
	now := time.Now().UTC()
	return tx.Exec(
		`INSERT INTO profiles
		 (id, user_id, display_name, language, timezone, communication_style, created_at, updated_at)
		 VALUES (?, ?, ?, 'id', 'Asia/Jakarta', 'adaptive', ?, ?)`,
		id, userID, displayName, now, now,
	).Error
}

// CreateSession inserts a session outside a transaction.
func (r *Repository) CreateSession(ctx context.Context, session *Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// CreateSessionTx inserts a session inside the supplied transaction.
func (r *Repository) CreateSessionTx(tx *gorm.DB, session *Session) error {
	return tx.Create(session).Error
}

// SessionForUpdate locks the session with the given refresh-token hash.
func (r *Repository) SessionForUpdate(tx *gorm.DB, refreshTokenHash string) (*Session, error) {
	var session Session
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("refresh_token_hash = ?", refreshTokenHash).
		Take(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// SessionPrincipal loads the session and user state required to authorize a
// protected request.
func (r *Repository) SessionPrincipal(ctx context.Context, sessionID string) (*SessionPrincipal, error) {
	var principal SessionPrincipal

	row := r.db.WithContext(ctx).Raw(
		`SELECT s.id, s.user_id, u.status, s.revoked_at, s.expires_at, u.deleted_at
		 FROM auth_sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.id = ?`,
		sessionID,
	).Row()

	err := row.Scan(
		&principal.SessionID,
		&principal.UserID,
		&principal.Status,
		&principal.RevokedAt,
		&principal.ExpiresAt,
		&principal.DeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}

	return &principal, nil
}

// RevokeSession marks one session as revoked. It is idempotent.
func (r *Repository) RevokeSession(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE auth_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), sessionID,
	).Error
}

// RevokeFamily revokes every active session in a token family.
func (r *Repository) RevokeFamily(tx *gorm.DB, familyID string) error {
	return tx.Exec(
		`UPDATE auth_sessions SET revoked_at = ? WHERE token_family_id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), familyID,
	).Error
}

// RevokeSessionTx marks one session as revoked inside a transaction.
func (r *Repository) RevokeSessionTx(tx *gorm.DB, sessionID string) error {
	return tx.Exec(
		`UPDATE auth_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), sessionID,
	).Error
}

// MarkReplaced records rotation on the previous session.
func (r *Repository) MarkReplaced(tx *gorm.DB, oldSessionID, newSessionID string) error {
	return tx.Exec(
		`UPDATE auth_sessions SET replaced_by = ?, last_used_at = ? WHERE id = ?`,
		newSessionID, time.Now().UTC(), oldSessionID,
	).Error
}

// CreateAuthTokenTx inserts a single-use token inside a transaction.
func (r *Repository) CreateAuthTokenTx(tx *gorm.DB, token *AuthToken) error {
	return tx.Create(token).Error
}

// FindAuthTokenTx locks and returns a matching token.
func (r *Repository) FindAuthTokenTx(tx *gorm.DB, purpose, tokenHash string) (*AuthToken, error) {
	var token AuthToken
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("purpose = ? AND token_hash = ?", purpose, tokenHash).
		Take(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// MarkAuthTokenUsedTx marks a token as consumed.
func (r *Repository) MarkAuthTokenUsedTx(tx *gorm.DB, tokenID string, now time.Time) error {
	return tx.Exec(
		`UPDATE auth_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		now, tokenID,
	).Error
}

// InvalidateAuthTokensTx invalidates all unused tokens for a purpose.
func (r *Repository) InvalidateAuthTokensTx(tx *gorm.DB, userID, purpose string, now time.Time) error {
	return tx.Exec(
		`UPDATE auth_tokens SET used_at = ? WHERE user_id = ? AND purpose = ? AND used_at IS NULL`,
		now, userID, purpose,
	).Error
}

// SetEmailVerifiedTx marks the user's email as verified.
func (r *Repository) SetEmailVerifiedTx(tx *gorm.DB, userID string, now time.Time) error {
	return tx.Exec(
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, ?), updated_at = ? WHERE id = ?`,
		now, now, userID,
	).Error
}

// UpdatePasswordTx replaces the password hash.
func (r *Repository) UpdatePasswordTx(tx *gorm.DB, userID, passwordHash string, now time.Time) error {
	return tx.Exec(
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, userID,
	).Error
}

// RevokeAllSessionsTx revokes every active session for a user.
func (r *Repository) RevokeAllSessionsTx(tx *gorm.DB, userID string, now time.Time) error {
	return tx.Exec(
		`UPDATE auth_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		now, userID,
	).Error
}

// ListSessions returns the user's refresh sessions, newest first.
func (r *Repository) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionView, error) {
	var sessions []SessionView
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, created_at, last_used_at, expires_at, revoked_at,
		        (id = ?::uuid) AS current
		 FROM auth_sessions
		 WHERE user_id = ?
		 ORDER BY created_at DESC, id DESC`,
		currentSessionID, userID,
	).Scan(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// RevokeOtherSessions revokes every session except the current one.
func (r *Repository) RevokeOtherSessions(ctx context.Context, userID, currentSessionID string, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE auth_sessions
		 SET revoked_at = ?
		 WHERE user_id = ? AND id <> ? AND revoked_at IS NULL`,
		now, userID, currentSessionID,
	)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// RevokeOwnedSession revokes one session owned by the user.
func (r *Repository) RevokeOwnedSession(ctx context.Context, userID, sessionID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE auth_sessions
		 SET revoked_at = ?
		 WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		now, sessionID, userID,
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
