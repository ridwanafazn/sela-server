package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repository reads and updates the current user's profile.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the user repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetMe returns the current user and profile.
func (r *Repository) GetMe(ctx context.Context, userID string) (*Me, error) {
	return getMe(r.db.WithContext(ctx), userID)
}

// UpdateMe applies a profile patch inside a transaction and returns the
// updated view.
func (r *Repository) UpdateMe(ctx context.Context, userID string, patch ProfilePatch) (*Me, error) {
	var updated *Me

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var (
			displayName        *string
			language           string
			timezone           string
			communicationStyle string
		)

		row := tx.Raw(
			`SELECT display_name, language, timezone, communication_style
			 FROM profiles WHERE user_id = ? FOR UPDATE`,
			userID,
		).Row()
		if err := row.Scan(&displayName, &language, &timezone, &communicationStyle); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		if patch.DisplayName.Set {
			displayName = patch.DisplayName.Value
		}
		if patch.Language.Set {
			language = *patch.Language.Value
		}
		if patch.Timezone.Set {
			timezone = *patch.Timezone.Value
		}
		if patch.CommunicationStyle.Set {
			communicationStyle = *patch.CommunicationStyle.Value
		}

		if err := tx.Exec(
			`UPDATE profiles
			 SET display_name = ?, language = ?, timezone = ?, communication_style = ?, updated_at = ?
			 WHERE user_id = ?`,
			displayName, language, timezone, communicationStyle, time.Now().UTC(), userID,
		).Error; err != nil {
			return err
		}

		result, err := getMe(tx, userID)
		if err != nil {
			return err
		}
		updated = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func getMe(db *gorm.DB, userID string) (*Me, error) {
	var me Me

	row := db.Raw(
		`SELECT u.id, u.email, u.email_verified_at, u.created_at,
		        p.display_name, p.language, p.timezone, p.communication_style
		 FROM users u
		 JOIN profiles p ON p.user_id = u.id
		 WHERE u.id = ?`,
		userID,
	).Row()

	if err := row.Scan(
		&me.UserID,
		&me.Email,
		&me.EmailVerifiedAt,
		&me.UserCreatedAt,
		&me.DisplayName,
		&me.Language,
		&me.Timezone,
		&me.CommunicationStyle,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}

	return &me, nil
}
