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
