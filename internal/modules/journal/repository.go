package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists journals and idempotency records.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the journal repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// ProfileTimezone returns the user's timezone, or an empty string when no
// profile exists.
func (r *Repository) ProfileTimezone(ctx context.Context, userID string) (string, error) {
	var timezone string
	err := r.db.WithContext(ctx).Raw(
		`SELECT timezone FROM profiles WHERE user_id = ?`, userID,
	).Row().Scan(&timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load profile timezone: %w", err)
	}
	return timezone, nil
}

// ClaimIdempotency inserts the key when unused. It reports whether this
// request won the claim.
func (r *Repository) ClaimIdempotency(tx *gorm.DB, record *IdempotencyRecord) (bool, error) {
	result := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "endpoint"},
			{Name: "key"},
		},
		DoNothing: true,
	}).Create(record)

	return result.RowsAffected == 1, result.Error
}

// IdempotencyRecord loads the stored record for a key.
func (r *Repository) IdempotencyRecord(tx *gorm.DB, userID, endpoint, key string) (*IdempotencyRecord, error) {
	var record IdempotencyRecord
	if err := tx.Where("user_id = ? AND endpoint = ? AND key = ?", userID, endpoint, key).
		Take(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// CompleteIdempotency stores the replayable response.
func (r *Repository) CompleteIdempotency(tx *gorm.DB, id string, status int, body []byte) error {
	return tx.Exec(
		`UPDATE idempotency_keys SET response_status = ?, response_body = ?::jsonb WHERE id = ?`,
		status, string(body), id,
	).Error
}

// Create inserts a journal inside the supplied transaction.
func (r *Repository) Create(tx *gorm.DB, journal *Journal) error {
	return tx.Exec(
		`INSERT INTO journals
		 (id, user_id, title, content, entry_date, status, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?::date, ?, ?, ?, ?)`,
		journal.ID,
		journal.UserID,
		journal.Title,
		journal.Content,
		journal.EntryDate.Format("2006-01-02"),
		journal.Status,
		journal.Version,
		journal.CreatedAt,
		journal.UpdatedAt,
	).Error
}

// Get returns one owned, non-deleted journal.
func (r *Repository) Get(ctx context.Context, userID, id string) (*Journal, error) {
	var journal Journal
	if err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		Take(&journal).Error; err != nil {
		return nil, err
	}
	return &journal, nil
}

// GetForUpdate locks one owned, non-deleted journal.
func (r *Repository) GetForUpdate(tx *gorm.DB, userID, id string) (*Journal, error) {
	var journal Journal
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		Take(&journal).Error; err != nil {
		return nil, err
	}
	return &journal, nil
}

// List returns up to limit+1 journals in stable order.
func (r *Repository) List(ctx context.Context, userID string, filter ListFilter, afterDate *time.Time, afterID string) ([]Journal, error) {
	query := `SELECT id, user_id, title, content, entry_date, status, version, created_at, updated_at
	          FROM journals
	          WHERE user_id = ? AND deleted_at IS NULL`
	args := []any{userID}

	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.From != nil {
		query += ` AND entry_date >= ?::date`
		args = append(args, filter.From.Format("2006-01-02"))
	}
	if filter.To != nil {
		query += ` AND entry_date <= ?::date`
		args = append(args, filter.To.Format("2006-01-02"))
	}
	if filter.Query != "" {
		query += ` AND search_vector @@ websearch_to_tsquery('simple', ?)`
		args = append(args, filter.Query)
	}
	if afterDate != nil {
		query += ` AND (entry_date, id) < (?::date, ?::uuid)`
		args = append(args, afterDate.Format("2006-01-02"), afterID)
	}

	query += ` ORDER BY entry_date DESC, id DESC LIMIT ?`
	args = append(args, filter.Limit+1)

	var journals []Journal
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&journals).Error; err != nil {
		return nil, fmt.Errorf("list journals: %w", err)
	}

	return journals, nil
}

// SaveTx writes the mutable journal fields and the current version.
func (r *Repository) SaveTx(tx *gorm.DB, journal *Journal) error {
	return tx.Exec(
		`UPDATE journals
		 SET title = ?, content = ?, entry_date = ?::date, status = ?, version = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		journal.Title,
		journal.Content,
		journal.EntryDate.Format("2006-01-02"),
		journal.Status,
		journal.Version,
		journal.UpdatedAt,
		journal.ID,
		journal.UserID,
	).Error
}

// SoftDeleteTx marks a journal deleted and advances its version.
func (r *Repository) SoftDeleteTx(tx *gorm.DB, id, userID string, version int, now time.Time) error {
	return tx.Exec(
		`UPDATE journals
		 SET deleted_at = ?, version = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, version, now, id, userID,
	).Error
}
