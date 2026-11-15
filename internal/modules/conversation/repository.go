package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

// Repository persists conversations, messages, and idempotency records.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the conversation repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn in a database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// CreateConversation inserts a conversation.
func (r *Repository) CreateConversation(ctx context.Context, conversation *Conversation) error {
	return r.db.WithContext(ctx).Create(conversation).Error
}

// GetConversation returns one owned, non-deleted conversation.
func (r *Repository) GetConversation(ctx context.Context, userID, id string) (*Conversation, error) {
	var conversation Conversation
	if err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		Take(&conversation).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

// ListConversations returns up to limit+1 conversations in stable order.
func (r *Repository) ListConversations(ctx context.Context, userID string, after *time.Time, afterID string, limit int) ([]Conversation, error) {
	query := `SELECT id, user_id, title, created_at, updated_at
	          FROM conversations
	          WHERE user_id = ? AND deleted_at IS NULL`
	args := []any{userID}

	if after != nil {
		query += ` AND (updated_at, id) < (?, ?::uuid)`
		args = append(args, *after, afterID)
	}

	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	var conversations []Conversation
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&conversations).Error; err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return conversations, nil
}

// SoftDeleteConversationTx marks a conversation deleted inside a transaction.
func (r *Repository) SoftDeleteConversationTx(tx *gorm.DB, userID, id string, now time.Time) (bool, error) {
	result := tx.Exec(
		`UPDATE conversations
		 SET deleted_at = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, now, id, userID,
	)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if err := tx.Exec(
		`UPDATE insight_evidence e
		 SET source_deleted_at = ?
		 FROM messages m
		 WHERE e.message_id = m.id
		   AND m.conversation_id = ?
		   AND e.source_deleted_at IS NULL`,
		now, id,
	).Error; err != nil {
		return false, err
	}
	return true, nil
}

// CreateMessageTx inserts a message inside a transaction. It reports whether
// the row was inserted; false means the idempotency key already existed.
func (r *Repository) CreateMessageTx(tx *gorm.DB, message *Message) (bool, error) {
	result := tx.Exec(
		`INSERT INTO messages
		 (id, conversation_id, user_id, role, content, idempotency_key, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?::uuid, ?::jsonb, ?)
		 ON CONFLICT (conversation_id, idempotency_key) DO NOTHING`,
		message.ID,
		message.ConversationID,
		message.UserID,
		message.Role,
		message.Content,
		message.IdempotencyKey,
		nullableJSON(message.Metadata),
		message.CreatedAt,
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// FindMessageByIdempotencyKey loads a previously persisted user message.
func (r *Repository) FindMessageByIdempotencyKey(tx *gorm.DB, conversationID, key string) (*Message, error) {
	var message Message
	if err := tx.Where("conversation_id = ? AND idempotency_key = ?", conversationID, key).
		Take(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

// ListMessages returns up to limit+1 messages in descending order.
func (r *Repository) ListMessages(ctx context.Context, conversationID string, before *time.Time, beforeID string, limit int) ([]Message, error) {
	query := `SELECT id, conversation_id, user_id, role, content, created_at
	          FROM messages
	          WHERE conversation_id = ?`
	args := []any{conversationID}

	if before != nil {
		query += ` AND (created_at, id) < (?, ?::uuid)`
		args = append(args, *before, beforeID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	var messages []Message
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&messages).Error; err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return messages, nil
}

// RecentMessages returns the newest messages in descending order.
func (r *Repository) RecentMessages(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	var messages []Message
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, conversation_id, user_id, role, content, created_at
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		conversationID, limit,
	).Scan(&messages).Error; err != nil {
		return nil, fmt.Errorf("recent messages: %w", err)
	}
	return messages, nil
}

// UpdateConversationTimestamp updates the conversation activity time.
func (r *Repository) UpdateConversationTimestamp(tx *gorm.DB, conversationID string, now time.Time) error {
	return tx.Exec(
		`UPDATE conversations SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		now, conversationID,
	).Error
}

// ProfilePreferences returns the user's language, communication style, and
// timezone.
func (r *Repository) ProfilePreferences(ctx context.Context, userID string) (string, string, string, error) {
	var language, style, timezone string
	err := r.db.WithContext(ctx).Raw(
		`SELECT language, communication_style, timezone FROM profiles WHERE user_id = ?`, userID,
	).Row().Scan(&language, &style, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "id", "adaptive", "Asia/Jakarta", nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("load profile preferences: %w", err)
	}
	return language, style, timezone, nil
}

// ClaimIdempotency inserts the key when unused.
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

// IdempotencyRecord loads a stored key.
func (r *Repository) IdempotencyRecord(tx *gorm.DB, userID, endpoint, key string) (*IdempotencyRecord, error) {
	var record IdempotencyRecord
	if err := tx.Where("user_id = ? AND endpoint = ? AND key = ?", userID, endpoint, key).
		Take(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// DeleteIdempotencyTx deletes a key inside a transaction.
func (r *Repository) DeleteIdempotencyTx(tx *gorm.DB, id string) error {
	return tx.Exec(`DELETE FROM idempotency_keys WHERE id = ?`, id).Error
}

// DeleteIdempotency deletes a key.
func (r *Repository) DeleteIdempotency(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Exec(`DELETE FROM idempotency_keys WHERE id = ?`, id).Error
}

// CompleteIdempotency stores the replayable response.
func (r *Repository) CompleteIdempotency(tx *gorm.DB, id string, status int, body []byte) error {
	return tx.Exec(
		`UPDATE idempotency_keys SET response_status = ?, response_body = ?::jsonb WHERE id = ?`,
		status, string(body), id,
	).Error
}

// NewID is a small helper used by the service.
func NewID() (string, error) {
	return ids.New()
}

func nullableJSON(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}
