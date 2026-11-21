package search

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repository runs per-type full-text queries.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the search repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type journalRow struct {
	ID     string  `gorm:"column:id"`
	Title  *string `gorm:"column:title"`
	Text   string  `gorm:"column:text"`
	Status string  `gorm:"column:status"`
	Date   string  `gorm:"column:result_date"`
	Score  float64 `gorm:"column:score"`
}

// Journals searches active and draft journals.
func (r *Repository) Journals(ctx context.Context, userID, query string, limit int) ([]Row, error) {
	var rows []journalRow
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, title, content AS text, status,
		        to_char(entry_date, 'YYYY-MM-DD') AS result_date,
		        ts_rank_cd(search_vector, websearch_to_tsquery('simple', ?)) AS score
		 FROM journals
		 WHERE user_id = ?
		   AND deleted_at IS NULL
		   AND search_vector @@ websearch_to_tsquery('simple', ?)
		 ORDER BY score DESC, entry_date DESC, id DESC
		 LIMIT ?`,
		query, userID, query, limit,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search journals: %w", err)
	}

	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		status := row.Status
		result = append(result, Row{
			ID:     row.ID,
			Title:  row.Title,
			Text:   row.Text,
			Date:   row.Date,
			Status: &status,
			Score:  row.Score,
		})
	}
	return result, nil
}

type memoryRow struct {
	ID          string  `gorm:"column:id"`
	Text        string  `gorm:"column:text"`
	Status      string  `gorm:"column:status"`
	Sensitivity string  `gorm:"column:sensitivity"`
	Date        string  `gorm:"column:result_date"`
	Score       float64 `gorm:"column:score"`
}

// Memories searches non-deleted personal memories.
func (r *Repository) Memories(ctx context.Context, userID, query string, limit int) ([]Row, error) {
	var rows []memoryRow
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, content AS text, status, sensitivity,
		        to_char(updated_at, 'YYYY-MM-DD') AS result_date,
		        ts_rank_cd(search_vector, websearch_to_tsquery('simple', ?)) AS score
		 FROM personal_memories
		 WHERE user_id = ?
		   AND deleted_at IS NULL
		   AND search_vector @@ websearch_to_tsquery('simple', ?)
		 ORDER BY score DESC, updated_at DESC, id DESC
		 LIMIT ?`,
		query, userID, query, limit,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}

	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		status := row.Status
		sensitivity := row.Sensitivity
		result = append(result, Row{
			ID:          row.ID,
			Text:        row.Text,
			Date:        row.Date,
			Status:      &status,
			Sensitivity: &sensitivity,
			Score:       row.Score,
		})
	}
	return result, nil
}

type insightRow struct {
	ID    string  `gorm:"column:id"`
	Title string  `gorm:"column:title"`
	Text  string  `gorm:"column:text"`
	Date  string  `gorm:"column:result_date"`
	Score float64 `gorm:"column:score"`
}

// Insights searches non-deleted insights.
func (r *Repository) Insights(ctx context.Context, userID, query string, limit int) ([]Row, error) {
	var rows []insightRow
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, title,
		        summary || ' ' || observation || ' ' || interpretation AS text,
		        to_char(created_at, 'YYYY-MM-DD') AS result_date,
		        ts_rank_cd(search_vector, websearch_to_tsquery('simple', ?)) AS score
		 FROM insights
		 WHERE user_id = ?
		   AND deleted_at IS NULL
		   AND search_vector @@ websearch_to_tsquery('simple', ?)
		 ORDER BY score DESC, created_at DESC, id DESC
		 LIMIT ?`,
		query, userID, query, limit,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search insights: %w", err)
	}

	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		title := row.Title
		result = append(result, Row{
			ID:    row.ID,
			Title: &title,
			Text:  row.Text,
			Date:  row.Date,
			Score: row.Score,
		})
	}
	return result, nil
}

type analysisRow struct {
	ID        string  `gorm:"column:id"`
	JournalID string  `gorm:"column:journal_id"`
	Title     *string `gorm:"column:title"`
	Text      string  `gorm:"column:text"`
	Date      string  `gorm:"column:result_date"`
	Score     float64 `gorm:"column:score"`
}

// Analyses searches analyses whose journal still exists.
func (r *Repository) Analyses(ctx context.Context, userID, query string, limit int) ([]Row, error) {
	var rows []analysisRow
	if err := r.db.WithContext(ctx).Raw(
		`SELECT a.id, a.journal_id, j.title,
		        a.summary || ' ' || a.observation || ' ' || a.interpretation AS text,
		        to_char(a.created_at, 'YYYY-MM-DD') AS result_date,
		        ts_rank_cd(a.search_vector, websearch_to_tsquery('simple', ?)) AS score
		 FROM journal_analyses a
		 JOIN journals j ON j.id = a.journal_id
		 WHERE a.user_id = ?
		   AND j.deleted_at IS NULL
		   AND a.search_vector @@ websearch_to_tsquery('simple', ?)
		 ORDER BY score DESC, a.created_at DESC, a.id DESC
		 LIMIT ?`,
		query, userID, query, limit,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search analyses: %w", err)
	}

	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		journalID := row.JournalID
		result = append(result, Row{
			ID:       row.ID,
			ParentID: &journalID,
			Title:    row.Title,
			Text:     row.Text,
			Date:     row.Date,
			Score:    row.Score,
		})
	}
	return result, nil
}
