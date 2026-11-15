package journal

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

// ActiveForBackfill returns active journals after the given ID for derived-work
// regeneration. Content stays in memory only long enough to compute its hash.
func (r *Repository) ActiveForBackfill(ctx context.Context, afterID string, limit int) ([]BackfillTarget, error) {
	query := `SELECT id, user_id, content
	          FROM journals
	          WHERE status = 'active' AND deleted_at IS NULL`
	args := []any{}
	if afterID != "" {
		query += ` AND id > ?::uuid`
		args = append(args, afterID)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit)

	var targets []BackfillTarget
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&targets).Error; err != nil {
		return nil, fmt.Errorf("list backfill journals: %w", err)
	}
	return targets, nil
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
	if err := tx.Exec(
		`UPDATE journals
		 SET deleted_at = ?, version = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, version, now, id, userID,
	).Error; err != nil {
		return err
	}
	return tx.Exec(
		`UPDATE insight_evidence
		 SET source_deleted_at = ?
		 WHERE journal_id = ? AND source_deleted_at IS NULL`,
		now, id,
	).Error
}

// GetByIDForProcessing returns a journal for the worker, including
// soft-deleted rows.
func (r *Repository) GetByIDForProcessing(ctx context.Context, id string) (*Journal, error) {
	var journal Journal
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&journal).Error; err != nil {
		return nil, err
	}
	return &journal, nil
}

// DeleteEmbeddingsTx removes derived vectors for a journal inside a
// transaction.
func (r *Repository) DeleteEmbeddingsTx(tx *gorm.DB, journalID string) error {
	return tx.Exec(`DELETE FROM journal_embeddings WHERE journal_id = ?`, journalID).Error
}

// DeleteEmbeddings removes derived vectors for a journal.
func (r *Repository) DeleteEmbeddings(ctx context.Context, journalID string) error {
	return r.db.WithContext(ctx).Exec(`DELETE FROM journal_embeddings WHERE journal_id = ?`, journalID).Error
}

// ReplaceEmbeddings atomically replaces all vectors for a journal.
func (r *Repository) ReplaceEmbeddings(
	ctx context.Context,
	journalID, userID string,
	chunks []Chunk,
	vectors [][]float32,
	model string,
) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("embedding count mismatch: %d chunks, %d vectors", len(chunks), len(vectors))
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.DeleteEmbeddingsTx(tx, journalID); err != nil {
			return err
		}

		now := time.Now().UTC()
		for index, chunk := range chunks {
			id, err := ids.New()
			if err != nil {
				return err
			}

			if err := tx.Exec(
				`INSERT INTO journal_embeddings
				 (id, journal_id, user_id, chunk_index, start_offset, end_offset, content_hash, embedding, embedding_model, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?::vector, ?, ?)`,
				id, journalID, userID, chunk.Index, chunk.StartOffset, chunk.EndOffset,
				chunk.ContentHash, vectorString(vectors[index]), model, now,
			).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// TextCandidates returns full-text search candidates for retrieval.
func (r *Repository) TextCandidates(ctx context.Context, userID, query string, limit int) ([]Candidate, error) {
	var candidates []Candidate
	err := r.db.WithContext(ctx).Raw(
		`SELECT j.id AS journal_id, j.title, j.content, j.entry_date,
		        ts_rank_cd(j.search_vector, websearch_to_tsquery('simple', ?)) AS score
		 FROM journals j
		 WHERE j.user_id = ? AND j.status = 'active' AND j.deleted_at IS NULL
		   AND j.search_vector @@ websearch_to_tsquery('simple', ?)
		 ORDER BY score DESC
		 LIMIT ?`,
		query, userID, query, limit,
	).Scan(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("text candidates: %w", err)
	}
	return candidates, nil
}

// VectorCandidates returns semantic candidates for the active embedding
// model, keeping the best-scoring chunk per journal.
func (r *Repository) VectorCandidates(ctx context.Context, userID, vector, model string, limit int) ([]Candidate, error) {
	var rows []Candidate
	err := r.db.WithContext(ctx).Raw(
		`WITH q AS (SELECT ?::vector AS v)
		 SELECT j.id AS journal_id, j.title, j.content, j.entry_date,
		        1 - (je.embedding <=> q.v) AS score
		 FROM journal_embeddings je
		 JOIN journals j ON j.id = je.journal_id
		 CROSS JOIN q
		 WHERE je.user_id = ? AND j.user_id = ?
		   AND je.embedding_model = ?
		   AND j.status = 'active' AND j.deleted_at IS NULL
		 ORDER BY je.embedding <=> q.v
		 LIMIT ?`,
		vector, userID, userID, model, limit,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("vector candidates: %w", err)
	}

	best := make(map[string]Candidate, len(rows))
	for _, row := range rows {
		existing, ok := best[row.JournalID]
		if !ok || row.Score > existing.Score {
			best[row.JournalID] = row
		}
	}

	candidates := make([]Candidate, 0, len(best))
	for _, candidate := range best {
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}
