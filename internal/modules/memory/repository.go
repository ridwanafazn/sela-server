package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

// Repository persists personal memories and their sources.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the memory repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn in a database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// JournalForExtraction returns a journal for the extraction worker.
func (r *Repository) JournalForExtraction(ctx context.Context, journalID string) (*JournalSource, error) {
	var entry JournalSource
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, title, content, status, entry_date, deleted_at
		 FROM journals
		 WHERE id = ?`,
		journalID,
	).Scan(&entry).Error; err != nil {
		return nil, fmt.Errorf("load extraction journal: %w", err)
	}
	if entry.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &entry, nil
}

// EligibleJournalIDs returns the subset of journal IDs owned by the user and
// eligible for extraction.
func (r *Repository) EligibleJournalIDs(ctx context.Context, userID string, journalIDs []string) (map[string]bool, error) {
	eligible := make(map[string]bool, len(journalIDs))
	if len(journalIDs) == 0 {
		return eligible, nil
	}

	var rows []string
	query := `SELECT id
	          FROM journals
	          WHERE user_id = ? AND status = 'active' AND deleted_at IS NULL
	            AND id IN (` + placeholders(len(journalIDs)) + `)`
	args := make([]any, 0, len(journalIDs)+1)
	args = append(args, userID)
	for _, id := range journalIDs {
		args = append(args, id)
	}
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("validate journal sources: %w", err)
	}
	for _, id := range rows {
		eligible[id] = true
	}
	return eligible, nil
}

// OwnedMemoryIDs returns the subset of memory IDs owned by the user.
func (r *Repository) OwnedMemoryIDs(ctx context.Context, userID string, memoryIDs []string) (map[string]bool, error) {
	owned := make(map[string]bool, len(memoryIDs))
	if len(memoryIDs) == 0 {
		return owned, nil
	}

	var rows []string
	query := `SELECT id FROM personal_memories
	          WHERE user_id = ? AND id IN (` + placeholders(len(memoryIDs)) + `)`
	args := make([]any, 0, len(memoryIDs)+1)
	args = append(args, userID)
	for _, id := range memoryIDs {
		args = append(args, id)
	}
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("validate memory conflicts: %w", err)
	}
	for _, id := range rows {
		owned[id] = true
	}
	return owned, nil
}

// FindByNormalizedContent returns the earliest matching non-deleted memory.
func (r *Repository) FindByNormalizedContent(ctx context.Context, userID, content string) (*Memory, error) {
	var memory Memory
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, content, category, status, confidence, source_count,
		        supersedes_memory_id, last_confirmed_at, created_at, updated_at, deleted_at
		 FROM personal_memories
		 WHERE user_id = ? AND deleted_at IS NULL
		   AND lower(regexp_replace(btrim(content), '\s+', ' ', 'g')) = lower(btrim(?))
		 ORDER BY CASE status
		            WHEN 'active' THEN 0
		            WHEN 'candidate' THEN 1
		            ELSE 2
		          END,
		          created_at
		 LIMIT 1`,
		userID, content,
	).Scan(&memory).Error; err != nil {
		return nil, fmt.Errorf("find duplicate memory: %w", err)
	}
	if memory.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &memory, nil
}

// Create inserts a candidate memory and its first journal source.
func (r *Repository) Create(ctx context.Context, memory *Memory, journalID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO personal_memories
			 (id, user_id, content, category, status, confidence, source_count,
			  supersedes_memory_id, last_confirmed_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0, ?, NULL, ?, ?)`,
			memory.ID,
			memory.UserID,
			memory.Content,
			memory.Category,
			memory.Status,
			memory.Confidence,
			memory.SupersedesMemoryID,
			memory.CreatedAt,
			memory.UpdatedAt,
		).Error; err != nil {
			return fmt.Errorf("insert memory: %w", err)
		}

		if _, err := r.addJournalSourceTx(tx, memory.ID, journalID, memory.UpdatedAt); err != nil {
			return err
		}
		return r.refreshSourceCountTx(tx, memory.ID, memory.UpdatedAt)
	})
}

// AddJournalSource attaches a journal to an existing memory when missing.
func (r *Repository) AddJournalSource(ctx context.Context, memoryID, journalID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		added, err := r.addJournalSourceTx(tx, memoryID, journalID, now)
		if err != nil {
			return err
		}
		if !added {
			return nil
		}
		return r.refreshSourceCountTx(tx, memoryID, now)
	})
}

func (r *Repository) addJournalSourceTx(tx *gorm.DB, memoryID, journalID string, now time.Time) (bool, error) {
	id, err := ids.New()
	if err != nil {
		return false, err
	}

	result := tx.Exec(
		`INSERT INTO memory_sources (id, memory_id, journal_id, message_id, source_excerpt, created_at)
		 SELECT ?, ?, ?, NULL, NULL, ?
		 WHERE NOT EXISTS (
		   SELECT 1 FROM memory_sources WHERE memory_id = ? AND journal_id = ?
		 )`,
		id, memoryID, journalID, now, memoryID, journalID,
	)
	if result.Error != nil {
		return false, fmt.Errorf("insert memory source: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) refreshSourceCountTx(tx *gorm.DB, memoryID string, now time.Time) error {
	return tx.Exec(
		`UPDATE personal_memories
		 SET source_count = (SELECT count(*) FROM memory_sources WHERE memory_id = ?),
		     updated_at = ?
		 WHERE id = ?`,
		memoryID, now, memoryID,
	).Error
}

// Get returns one owned, non-deleted memory.
func (r *Repository) Get(ctx context.Context, userID, id string) (*Memory, error) {
	var memory Memory
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, content, category, status, confidence, source_count,
		        supersedes_memory_id, last_confirmed_at, created_at, updated_at, deleted_at
		 FROM personal_memories
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		id, userID,
	).Scan(&memory).Error; err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	if memory.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &memory, nil
}

// GetForUpdate locks one owned, non-deleted memory.
func (r *Repository) GetForUpdate(tx *gorm.DB, userID, id string) (*Memory, error) {
	var memory Memory
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Raw(
		`SELECT id, user_id, content, category, status, confidence, source_count,
		        supersedes_memory_id, last_confirmed_at, created_at, updated_at, deleted_at
		 FROM personal_memories
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		id, userID,
	).Scan(&memory).Error; err != nil {
		return nil, fmt.Errorf("lock memory: %w", err)
	}
	if memory.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &memory, nil
}

// Save writes the mutable review fields inside a transaction.
func (r *Repository) Save(tx *gorm.DB, memory *Memory) error {
	return tx.Exec(
		`UPDATE personal_memories
		 SET content = ?, category = ?, status = ?, last_confirmed_at = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		memory.Content,
		memory.Category,
		memory.Status,
		memory.LastConfirmedAt,
		memory.UpdatedAt,
		memory.ID,
		memory.UserID,
	).Error
}

// SoftDelete marks a memory deleted.
func (r *Repository) SoftDelete(tx *gorm.DB, memory *Memory, now time.Time) error {
	return tx.Exec(
		`UPDATE personal_memories
		 SET status = 'deleted', deleted_at = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, now, memory.ID, memory.UserID,
	).Error
}

// List returns up to limit+1 memories in stable order.
func (r *Repository) List(
	ctx context.Context,
	userID string,
	status, category string,
	afterUpdatedAt *time.Time,
	afterID string,
	limit int,
) ([]Memory, error) {
	query := `SELECT id, user_id, content, category, status, confidence, source_count,
	                 supersedes_memory_id, last_confirmed_at, created_at, updated_at, deleted_at
	          FROM personal_memories
	          WHERE user_id = ? AND deleted_at IS NULL AND status <> 'deleted'`
	args := []any{userID}

	if status != "" && status != "all" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	if afterUpdatedAt != nil {
		query += ` AND (updated_at, id) < (?, ?::uuid)`
		args = append(args, *afterUpdatedAt, afterID)
	}

	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	var memories []Memory
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&memories).Error; err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	return memories, nil
}

// Sources returns provenance rows for a memory.
func (r *Repository) Sources(ctx context.Context, memoryID string) ([]SourceView, error) {
	var sources []SourceView
	if err := r.db.WithContext(ctx).Raw(
		`SELECT ms.id,
		        CASE WHEN ms.journal_id IS NOT NULL THEN 'journal' ELSE 'message' END AS source_type,
		        COALESCE(ms.journal_id, ms.message_id) AS source_id,
		        COALESCE(j.entry_date::timestamptz, m.created_at) AS source_date,
		        ms.source_excerpt AS excerpt,
		        CASE
		          WHEN ms.journal_id IS NOT NULL THEN (j.id IS NULL OR j.deleted_at IS NOT NULL)
		          ELSE (m.id IS NULL)
		        END AS source_deleted
		 FROM memory_sources ms
		 LEFT JOIN journals j ON j.id = ms.journal_id
		 LEFT JOIN messages m ON m.id = ms.message_id
		 WHERE ms.memory_id = ?
		 ORDER BY ms.created_at, ms.id`,
		memoryID,
	).Scan(&sources).Error; err != nil {
		return nil, fmt.Errorf("list memory sources: %w", err)
	}
	return sources, nil
}

// RetrievalCandidates returns memories eligible for Companion context.
func (r *Repository) RetrievalCandidates(ctx context.Context, userID string, limit int) ([]RetrievedMemory, error) {
	var memories []RetrievedMemory
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, content, category, status, COALESCE(confidence, 0) AS confidence, updated_at
		 FROM personal_memories
		 WHERE user_id = ? AND deleted_at IS NULL
		   AND status IN ('active', 'candidate')
		   AND (status = 'active' OR source_count > 0)
		 ORDER BY (status = 'active') DESC, confidence DESC NULLS LAST, updated_at DESC
		 LIMIT ?`,
		userID, limit,
	).Scan(&memories).Error; err != nil {
		return nil, fmt.Errorf("load memory retrieval candidates: %w", err)
	}
	return memories, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
