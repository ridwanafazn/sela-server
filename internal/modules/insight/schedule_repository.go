package insight

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

// GetSchedule returns one user's schedule.
func (r *Repository) GetSchedule(ctx context.Context, userID string) (*Schedule, error) {
	var schedule Schedule
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, cadence, timezone, enabled, next_run_at, last_run_at,
		        created_at, updated_at
		 FROM insight_schedules
		 WHERE user_id = ?`,
		userID,
	).Scan(&schedule).Error; err != nil {
		return nil, fmt.Errorf("get insight schedule: %w", err)
	}
	if schedule.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &schedule, nil
}

// UpsertSchedule creates or updates one user's schedule.
func (r *Repository) UpsertSchedule(
	ctx context.Context,
	userID, cadence, timezone string,
	enabled bool,
	nextRunAt, now time.Time,
) error {
	id, err := ids.New()
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO insight_schedules
		 (id, user_id, cadence, timezone, enabled, next_run_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		   cadence = EXCLUDED.cadence,
		   timezone = EXCLUDED.timezone,
		   enabled = EXCLUDED.enabled,
		   next_run_at = EXCLUDED.next_run_at,
		   updated_at = EXCLUDED.updated_at`,
		id, userID, cadence, timezone, enabled, nextRunAt, now, now,
	).Error
}

// DisableSchedule turns off one user's schedule.
func (r *Repository) DisableSchedule(ctx context.Context, userID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE insight_schedules
		 SET enabled = false, updated_at = ?
		 WHERE user_id = ? AND enabled = true`,
		now, userID,
	)
	if result.Error != nil {
		return false, fmt.Errorf("disable insight schedule: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// DueSchedules returns enabled schedules whose next run is due.
func (r *Repository) DueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	var schedules []Schedule
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, cadence, timezone, enabled, next_run_at, last_run_at,
		        created_at, updated_at
		 FROM insight_schedules
		 WHERE enabled = true AND next_run_at <= ?
		 ORDER BY next_run_at
		 LIMIT ?`,
		now, limit,
	).Scan(&schedules).Error; err != nil {
		return nil, fmt.Errorf("load due insight schedules: %w", err)
	}
	return schedules, nil
}

// AdvanceSchedule moves a schedule forward. The old next-run value is part of
// the predicate so two scheduler instances cannot advance the same run twice.
func (r *Repository) AdvanceSchedule(
	ctx context.Context,
	scheduleID string,
	oldNextRun, lastRun, nextRun time.Time,
) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE insight_schedules
		 SET last_run_at = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ? AND next_run_at = ? AND enabled = true`,
		lastRun, nextRun, lastRun, scheduleID, oldNextRun,
	).Error
}
