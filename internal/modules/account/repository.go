package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repository persists account lifecycle state and purge operations.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the account repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Account returns the authentication identity for re-authentication.
func (r *Repository) Account(ctx context.Context, userID string) (*Account, error) {
	var account Account
	row := r.db.WithContext(ctx).Raw(
		`SELECT id, password_hash, status, deleted_at FROM users WHERE id = ?`,
		userID,
	).Row()
	if err := row.Scan(&account.ID, &account.PasswordHash, &account.Status, &account.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("load account: %w", err)
	}
	return &account, nil
}

// BeginDeletion deactivates the account, revokes every session, and enqueues
// the purge job in one transaction.
func (r *Repository) BeginDeletion(
	ctx context.Context,
	userID string,
	now time.Time,
	enqueue func(tx *gorm.DB) error,
) (bool, error) {
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(
			`UPDATE users
			 SET status = 'deletion_pending', deletion_requested_at = ?, updated_at = ?
			 WHERE id = ? AND status = 'active' AND deleted_at IS NULL`,
			now, now, userID,
		)
		if result.Error != nil {
			return fmt.Errorf("deactivate account: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		changed = true

		if err := tx.Exec(
			`UPDATE auth_sessions
			 SET revoked_at = ?
			 WHERE user_id = ? AND revoked_at IS NULL`,
			now, userID,
		).Error; err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}

		return enqueue(tx)
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// Purge hard-deletes the user. Foreign-key cascades remove user-owned data.
func (r *Repository) Purge(ctx context.Context, userID string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`DELETE FROM users WHERE id = ?`, userID)
	if result.Error != nil {
		return false, fmt.Errorf("purge account: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
