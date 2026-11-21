package insight

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

// Repository persists insights, evidence, feedback, and generation inputs.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the insight repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn in a database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// Profile returns the user's language and timezone.
func (r *Repository) Profile(ctx context.Context, userID string) (string, string, error) {
	var language, timezone string
	err := r.db.WithContext(ctx).Raw(
		`SELECT language, timezone FROM profiles WHERE user_id = ?`, userID,
	).Row().Scan(&language, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "id", "Asia/Jakarta", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load insight profile: %w", err)
	}
	return language, timezone, nil
}

// Create inserts an insight and its evidence. It returns false when another
// transaction already created the insight for the same job.
func (r *Repository) Create(ctx context.Context, insight *Insight, evidence []Evidence) (bool, error) {
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(
			`INSERT INTO insights
			 (id, user_id, job_id, title, summary, observation, interpretation, hypothesis,
			  confidence, confidence_label, reflection_question, suggested_direction,
			  period_start, period_end, generation_type, model, prompt_version, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::date, ?::date, ?, ?, ?, ?, ?)
			 ON CONFLICT (job_id) WHERE job_id IS NOT NULL DO NOTHING`,
			insight.ID,
			insight.UserID,
			insight.JobID,
			insight.Title,
			insight.Summary,
			insight.Observation,
			insight.Interpretation,
			insight.Hypothesis,
			insight.Confidence,
			insight.ConfidenceLabel,
			insight.ReflectionQuestion,
			insight.SuggestedDirection,
			dateValue(insight.PeriodStart),
			dateValue(insight.PeriodEnd),
			insight.GenerationType,
			insight.Model,
			insight.PromptVersion,
			insight.CreatedAt,
			insight.UpdatedAt,
		)
		if result.Error != nil {
			return fmt.Errorf("insert insight: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		created = true

		for index := range evidence {
			item := &evidence[index]
			item.InsightID = insight.ID
			if item.ID == "" {
				id, err := ids.New()
				if err != nil {
					return err
				}
				item.ID = id
			}
			if err := tx.Exec(
				`INSERT INTO insight_evidence
				 (id, insight_id, journal_id, message_id, memory_id, note, source_deleted_at, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
				item.ID, item.InsightID, item.JournalID, item.MessageID, item.MemoryID,
				item.Note, item.CreatedAt,
			).Error; err != nil {
				return fmt.Errorf("insert insight evidence: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// GetByJobID returns the insight produced by a job, including deleted rows.
func (r *Repository) GetByJobID(ctx context.Context, jobID string) (*Insight, error) {
	var insight Insight
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, job_id, title, summary, observation, interpretation, hypothesis,
		        confidence, confidence_label, reflection_question, suggested_direction,
		        period_start, period_end, generation_type, model, prompt_version,
		        created_at, updated_at, deleted_at
		 FROM insights
		 WHERE job_id = ?
		 LIMIT 1`,
		jobID,
	).Scan(&insight).Error; err != nil {
		return nil, fmt.Errorf("load insight by job: %w", err)
	}
	if insight.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &insight, nil
}

// Get returns one owned, non-deleted insight.
func (r *Repository) Get(ctx context.Context, userID, insightID string) (*Insight, error) {
	var insight Insight
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, job_id, title, summary, observation, interpretation, hypothesis,
		        confidence, confidence_label, reflection_question, suggested_direction,
		        period_start, period_end, generation_type, model, prompt_version,
		        created_at, updated_at, deleted_at
		 FROM insights
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		insightID, userID,
	).Scan(&insight).Error; err != nil {
		return nil, fmt.Errorf("get insight: %w", err)
	}
	if insight.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &insight, nil
}

type listRow struct {
	ID                 string     `gorm:"column:id"`
	Title              string     `gorm:"column:title"`
	Summary            string     `gorm:"column:summary"`
	Confidence         float64    `gorm:"column:confidence"`
	ConfidenceLabel    string     `gorm:"column:confidence_label"`
	SuggestedDirection string     `gorm:"column:suggested_direction"`
	PeriodStart        *time.Time `gorm:"column:period_start"`
	PeriodEnd          *time.Time `gorm:"column:period_end"`
	GenerationType     string     `gorm:"column:generation_type"`
	EvidenceCount      int        `gorm:"column:evidence_count"`
	FeedbackStatus     string     `gorm:"column:feedback_status"`
	FeedbackNote       *string    `gorm:"column:feedback_note"`
	FeedbackUpdatedAt  *time.Time `gorm:"column:feedback_updated_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
}

// List returns up to limit+1 insight summaries.
func (r *Repository) List(
	ctx context.Context,
	userID, period string,
	afterCreatedAt *time.Time,
	afterID string,
	limit int,
) ([]ListItem, error) {
	query := `SELECT i.id, i.title, i.summary, i.confidence, i.confidence_label,
	                 i.suggested_direction, i.period_start, i.period_end, i.generation_type,
	                 (SELECT count(*) FROM insight_evidence e WHERE e.insight_id = i.id) AS evidence_count,
	                 COALESCE((
	                   SELECT CASE f.action
	                     WHEN 'confirm' THEN 'confirmed'
	                     WHEN 'correct' THEN 'corrected'
	                     WHEN 'reject' THEN 'rejected'
	                   END
	                   FROM insight_feedback f
	                   WHERE f.insight_id = i.id
	                   ORDER BY f.created_at DESC, f.id DESC
	                   LIMIT 1
	                 ), 'none') AS feedback_status,
	                 (SELECT f.note FROM insight_feedback f
	                  WHERE f.insight_id = i.id
	                  ORDER BY f.created_at DESC, f.id DESC LIMIT 1) AS feedback_note,
	                 (SELECT f.created_at FROM insight_feedback f
	                  WHERE f.insight_id = i.id
	                  ORDER BY f.created_at DESC, f.id DESC LIMIT 1) AS feedback_updated_at,
	                 i.created_at
	          FROM insights i
	          WHERE i.user_id = ? AND i.deleted_at IS NULL`
	args := []any{userID}

	if period != "" && period != "all" {
		query += ` AND i.generation_type = ?`
		args = append(args, period)
	}
	if afterCreatedAt != nil {
		query += ` AND (i.created_at, i.id) < (?, ?::uuid)`
		args = append(args, *afterCreatedAt, afterID)
	}
	query += ` ORDER BY i.created_at DESC, i.id DESC LIMIT ?`
	args = append(args, limit+1)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list insights: %w", err)
	}

	items := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ListItem{
			ID:                 row.ID,
			Title:              row.Title,
			Summary:            row.Summary,
			Confidence:         row.Confidence,
			ConfidenceLabel:    row.ConfidenceLabel,
			SuggestedDirection: row.SuggestedDirection,
			PeriodStart:        dateString(row.PeriodStart),
			PeriodEnd:          dateString(row.PeriodEnd),
			GenerationType:     row.GenerationType,
			EvidenceCount:      row.EvidenceCount,
			Feedback: FeedbackView{
				Status:    row.FeedbackStatus,
				Note:      row.FeedbackNote,
				UpdatedAt: row.FeedbackUpdatedAt,
			},
			CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

// Evidence returns the provenance for an insight.
func (r *Repository) Evidence(ctx context.Context, insightID string) ([]EvidenceView, error) {
	var rows []EvidenceView
	if err := r.db.WithContext(ctx).Raw(
		`SELECT e.id,
		        CASE
		          WHEN e.journal_id IS NOT NULL THEN 'journal'
		          WHEN e.message_id IS NOT NULL THEN 'message'
		          ELSE 'memory'
		        END AS source_type,
		        COALESCE(e.journal_id, e.message_id, e.memory_id) AS source_id,
		        e.note,
		        COALESCE(j.entry_date::timestamptz, m.created_at, pm.updated_at) AS source_date,
		        (CASE
		          WHEN e.journal_id IS NOT NULL THEN (j.id IS NULL OR j.deleted_at IS NOT NULL)
		          WHEN e.message_id IS NOT NULL THEN (m.id IS NULL OR c.deleted_at IS NOT NULL)
		          ELSE (pm.id IS NULL OR pm.deleted_at IS NOT NULL)
		        END OR e.source_deleted_at IS NOT NULL) AS source_deleted
		 FROM insight_evidence e
		 LEFT JOIN journals j ON j.id = e.journal_id
		 LEFT JOIN messages m ON m.id = e.message_id
		 LEFT JOIN conversations c ON c.id = m.conversation_id
		 LEFT JOIN personal_memories pm ON pm.id = e.memory_id
		 WHERE e.insight_id = ?
		 ORDER BY e.created_at, e.id`,
		insightID,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list insight evidence: %w", err)
	}
	for index := range rows {
		if rows[index].SourceDeleted {
			rows[index].Note = "This source was deleted."
		}
	}
	if rows == nil {
		rows = []EvidenceView{}
	}
	return rows, nil
}

// LatestFeedback returns the newest feedback or a none default.
func (r *Repository) LatestFeedback(ctx context.Context, insightID string) (FeedbackView, error) {
	var status string
	var note *string
	var updatedAt *time.Time
	err := r.db.WithContext(ctx).Raw(
		`SELECT CASE action
		          WHEN 'confirm' THEN 'confirmed'
		          WHEN 'correct' THEN 'corrected'
		          WHEN 'reject' THEN 'rejected'
		        END,
		        note, created_at
		 FROM insight_feedback
		 WHERE insight_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		insightID,
	).Row().Scan(&status, &note, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FeedbackView{Status: FeedbackNone}, nil
	}
	if err != nil {
		return FeedbackView{}, fmt.Errorf("load insight feedback: %w", err)
	}
	return FeedbackView{Status: status, Note: note, UpdatedAt: updatedAt}, nil
}

// AddFeedback appends a feedback record.
func (r *Repository) AddFeedback(ctx context.Context, feedback *Feedback) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO insight_feedback (id, insight_id, user_id, action, note, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		feedback.ID, feedback.InsightID, feedback.UserID, feedback.Action,
		feedback.Note, feedback.CreatedAt,
	).Error
}

// SoftDelete marks an insight deleted.
func (r *Repository) SoftDelete(ctx context.Context, userID, insightID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE insights
		 SET deleted_at = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, now, insightID, userID,
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// EligibleJournals returns active journals in the insight period.
func (r *Repository) EligibleJournals(ctx context.Context, userID string, startDate, endDate time.Time, limit int) ([]SourceRecord, error) {
	var records []SourceRecord
	if err := r.db.WithContext(ctx).Raw(
		`SELECT 'journal' AS type, id, entry_date::timestamptz AS date, content, title, NULL AS category
		 FROM journals
		 WHERE user_id = ? AND status = 'active' AND deleted_at IS NULL
		   AND entry_date BETWEEN ?::date AND ?::date
		 ORDER BY entry_date DESC, id DESC
		 LIMIT ?`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), limit,
	).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("load insight journals: %w", err)
	}
	return records, nil
}

// EligibleMessages returns user messages in non-deleted conversations.
func (r *Repository) EligibleMessages(ctx context.Context, userID string, start, end time.Time, limit int) ([]SourceRecord, error) {
	var records []SourceRecord
	if err := r.db.WithContext(ctx).Raw(
		`SELECT 'message' AS type, m.id, m.created_at AS date, m.content, NULL AS title, NULL AS category
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.user_id = ? AND m.role = 'user' AND c.deleted_at IS NULL
		   AND m.created_at >= ? AND m.created_at < ?
		 ORDER BY m.created_at DESC, m.id DESC
		 LIMIT ?`,
		userID, start, end, limit,
	).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("load insight messages: %w", err)
	}
	return records, nil
}

// EligibleMemories returns confirmed memories for insight context.
func (r *Repository) EligibleMemories(ctx context.Context, userID string, limit int) ([]SourceRecord, error) {
	var records []SourceRecord
	if err := r.db.WithContext(ctx).Raw(
		`SELECT 'memory' AS type, id, updated_at AS date, content, NULL AS title, category
		 FROM personal_memories m
		 WHERE user_id = ? AND status = 'active' AND deleted_at IS NULL
		   AND (
		     sensitivity = 'normal'
		     OR EXISTS (
		       SELECT 1 FROM memory_consents mc
		       WHERE mc.user_id = m.user_id
		         AND mc.scope = 'sensitive_memory'
		         AND mc.granted = true
		         AND mc.revoked_at IS NULL
		     )
		   )
		 ORDER BY updated_at DESC, id DESC
		 LIMIT ?`,
		userID, limit,
	).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("load insight memories: %w", err)
	}
	return records, nil
}

func dateValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format("2006-01-02")
}
