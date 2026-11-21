package journalanalysis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/job"
)

// Repository persists per-entry journal analyses.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the journal analysis repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ProfileLanguage returns the user's preferred output language.
func (r *Repository) ProfileLanguage(ctx context.Context, userID string) (string, error) {
	var language string
	err := r.db.WithContext(ctx).Raw(
		`SELECT language FROM profiles WHERE user_id = ?`, userID,
	).Row().Scan(&language)
	if errors.Is(err, sql.ErrNoRows) {
		return "id", nil
	}
	if err != nil {
		return "", fmt.Errorf("load analysis profile: %w", err)
	}
	return language, nil
}

// GetByJobID returns the analysis produced by a job.
func (r *Repository) GetByJobID(ctx context.Context, jobID string) (*Analysis, error) {
	var analysis Analysis
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, journal_id, journal_version, job_id, summary,
		        observation, interpretation, hypothesis, confidence,
		        confidence_label, reflection_question, suggested_direction,
		        evidence_note, model, prompt_version, created_at
		 FROM journal_analyses
		 WHERE job_id = ?
		 LIMIT 1`,
		jobID,
	).Scan(&analysis).Error; err != nil {
		return nil, fmt.Errorf("load analysis by job: %w", err)
	}
	if analysis.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &analysis, nil
}

// GetByJournalVersion returns the analysis for one journal version.
func (r *Repository) GetByJournalVersion(
	ctx context.Context,
	userID, journalID string,
	version int,
) (*Analysis, error) {
	var analysis Analysis
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, journal_id, journal_version, job_id, summary,
		        observation, interpretation, hypothesis, confidence,
		        confidence_label, reflection_question, suggested_direction,
		        evidence_note, model, prompt_version, created_at
		 FROM journal_analyses
		 WHERE user_id = ? AND journal_id = ? AND journal_version = ?
		 LIMIT 1`,
		userID, journalID, version,
	).Scan(&analysis).Error; err != nil {
		return nil, fmt.Errorf("load analysis by journal version: %w", err)
	}
	if analysis.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &analysis, nil
}

type latestRow struct {
	ID                 string    `gorm:"column:id"`
	JournalID          string    `gorm:"column:journal_id"`
	JournalVersion     int       `gorm:"column:journal_version"`
	CurrentVersion     int       `gorm:"column:current_version"`
	EntryDate          time.Time `gorm:"column:entry_date"`
	Summary            string    `gorm:"column:summary"`
	Observation        string    `gorm:"column:observation"`
	Interpretation     string    `gorm:"column:interpretation"`
	Hypothesis         *string   `gorm:"column:hypothesis"`
	Confidence         float64   `gorm:"column:confidence"`
	ConfidenceLabel    string    `gorm:"column:confidence_label"`
	ReflectionQuestion string    `gorm:"column:reflection_question"`
	SuggestedDirection string    `gorm:"column:suggested_direction"`
	EvidenceNote       string    `gorm:"column:evidence_note"`
	Model              *string   `gorm:"column:model"`
	PromptVersion      *string   `gorm:"column:prompt_version"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

// GetLatest returns the newest analysis for one owned, non-deleted journal.
func (r *Repository) GetLatest(ctx context.Context, userID, journalID string) (*latestRow, error) {
	var row latestRow
	if err := r.db.WithContext(ctx).Raw(
		`SELECT a.id, a.journal_id, a.journal_version, j.version AS current_version,
		        j.entry_date, a.summary, a.observation, a.interpretation,
		        a.hypothesis, a.confidence, a.confidence_label,
		        a.reflection_question, a.suggested_direction, a.evidence_note,
		        a.model, a.prompt_version, a.created_at
		 FROM journal_analyses a
		 JOIN journals j ON j.id = a.journal_id
		 WHERE a.user_id = ? AND a.journal_id = ? AND j.deleted_at IS NULL
		 ORDER BY a.created_at DESC, a.id DESC
		 LIMIT 1`,
		userID, journalID,
	).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("load latest analysis: %w", err)
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

// Create inserts an analysis. It returns false when the journal version
// already has one.
func (r *Repository) Create(ctx context.Context, analysis *Analysis) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		`INSERT INTO journal_analyses
		 (id, user_id, journal_id, journal_version, job_id, summary,
		  observation, interpretation, hypothesis, confidence, confidence_label,
		  reflection_question, suggested_direction, evidence_note, model,
		  prompt_version, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (journal_id, journal_version) DO NOTHING`,
		analysis.ID,
		analysis.UserID,
		analysis.JournalID,
		analysis.JournalVersion,
		analysis.JobID,
		analysis.Summary,
		analysis.Observation,
		analysis.Interpretation,
		analysis.Hypothesis,
		analysis.Confidence,
		analysis.ConfidenceLabel,
		analysis.ReflectionQuestion,
		analysis.SuggestedDirection,
		analysis.EvidenceNote,
		analysis.Model,
		analysis.PromptVersion,
		analysis.CreatedAt,
	)
	if result.Error != nil {
		return false, fmt.Errorf("insert journal analysis: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

// FindActiveJob returns a pending, processing, or retryable analysis job for
// the journal version.
func (r *Repository) FindActiveJob(
	ctx context.Context,
	userID, journalID string,
	version int,
) (string, error) {
	var jobID string
	err := r.db.WithContext(ctx).Raw(
		`SELECT id
		 FROM scheduled_jobs
		 WHERE user_id = ?
		   AND job_type = ?
		   AND status IN (?, ?, ?)
		   AND payload->>'journal_id' = ?
		   AND (payload->>'journal_version')::int = ?
		 ORDER BY created_at
		 LIMIT 1`,
		userID,
		job.TypeJournalAnalysis,
		job.StatusPending,
		job.StatusProcessing,
		job.StatusRetry,
		journalID,
		version,
	).Row().Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", gorm.ErrRecordNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find active analysis job: %w", err)
	}
	return jobID, nil
}

// DeleteForJournal removes every stored analysis and cancels pending jobs.
func (r *Repository) DeleteForJournal(
	ctx context.Context,
	userID, journalID string,
	now time.Time,
) (bool, error) {
	found := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleted := tx.Exec(
			`DELETE FROM journal_analyses WHERE user_id = ? AND journal_id = ?`,
			userID, journalID,
		)
		if deleted.Error != nil {
			return fmt.Errorf("delete journal analyses: %w", deleted.Error)
		}
		if deleted.RowsAffected > 0 {
			found = true
		}

		canceled := tx.Exec(
			`UPDATE scheduled_jobs
			 SET status = ?, last_error = ?, updated_at = ?
			 WHERE user_id = ?
			   AND job_type = ?
			   AND status IN (?, ?)
			   AND payload->>'journal_id' = ?`,
			job.StatusFailed,
			"Canceled because the journal analysis was deleted.",
			now,
			userID,
			job.TypeJournalAnalysis,
			job.StatusPending,
			job.StatusRetry,
			journalID,
		)
		if canceled.Error != nil {
			return fmt.Errorf("cancel analysis jobs: %w", canceled.Error)
		}
		if canceled.RowsAffected > 0 {
			found = true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}
