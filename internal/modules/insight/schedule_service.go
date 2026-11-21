package insight

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

const scheduleFanOutLimit = 100

// ScheduleService manages weekly and monthly reflection schedules.
type ScheduleService struct {
	repo  *Repository
	queue *job.Queue
	usage *aiusage.Service
	log   *slog.Logger
	now   func() time.Time
}

// NewScheduleService creates the schedule service.
func NewScheduleService(
	repo *Repository,
	queue *job.Queue,
	usage *aiusage.Service,
	log *slog.Logger,
) *ScheduleService {
	if log == nil {
		log = slog.Default()
	}
	return &ScheduleService{
		repo:  repo,
		queue: queue,
		usage: usage,
		log:   log,
		now:   time.Now,
	}
}

// Get returns the current schedule or a disabled view when none exists.
func (s *ScheduleService) Get(ctx context.Context, userID string) (*ScheduleView, error) {
	_, timezone, err := s.repo.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}

	schedule, err := s.repo.GetSchedule(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		schedule = nil
	}

	view := scheduleView(schedule, timezone)
	if view.Enabled {
		view.BudgetBlocked = s.budgetBlocked(ctx, userID)
	}
	return view, nil
}

// Set creates, updates, or disables a schedule.
func (s *ScheduleService) Set(ctx context.Context, userID string, input ScheduleInput) (*ScheduleView, error) {
	if !input.Enabled {
		return s.Disable(ctx, userID)
	}

	cadence := strings.ToLower(strings.TrimSpace(input.Cadence))
	if cadence != CadenceWeekly && cadence != CadenceMonthly {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "cadence",
			Code:    "INVALID_VALUE",
			Message: "Cadence must be weekly or monthly.",
		})
	}

	_, timezone, err := s.repo.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}

	now := s.now().UTC()
	nextRunAt := nextRun(now, location, cadence)

	existing, err := s.repo.GetSchedule(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && existing.Enabled && existing.Cadence == cadence && existing.Timezone == timezone {
		nextRunAt = existing.NextRunAt
	}

	if err := s.repo.UpsertSchedule(ctx, userID, cadence, timezone, true, nextRunAt, now); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID)
}

// Disable turns off the current schedule.
func (s *ScheduleService) Disable(ctx context.Context, userID string) (*ScheduleView, error) {
	now := s.now().UTC()
	if _, err := s.repo.DisableSchedule(ctx, userID, now); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID)
}

// FanOutDue enqueues one insight generation job for each due schedule and
// advances the schedule. It is safe to run repeatedly.
func (s *ScheduleService) FanOutDue(ctx context.Context) (FanOutResult, error) {
	var result FanOutResult
	if s.queue == nil {
		return result, apierrors.Internal()
	}

	now := s.now().UTC()
	schedules, err := s.repo.DueSchedules(ctx, now, scheduleFanOutLimit)
	if err != nil {
		return result, err
	}
	result.Due = len(schedules)

	for _, schedule := range schedules {
		location, err := time.LoadLocation(schedule.Timezone)
		if err != nil {
			location = time.UTC
			s.log.WarnContext(ctx, "scheduled insight timezone fallback",
				slog.String("schedule_id", schedule.ID),
				slog.String("timezone", schedule.Timezone),
			)
		}

		startDate, endDate := scheduledPeriod(schedule.Cadence, schedule.NextRunAt, location)
		idempotencyKey := scheduleJobKey(schedule.ID, startDate, endDate)
		payload := JobPayload{
			Period:         schedule.Cadence,
			StartDate:      startDate,
			EndDate:        endDate,
			GenerationType: schedule.Cadence,
			RequestHash:    requestHash(schedule.Cadence, &startDate, &endDate, ""),
			ScheduleID:     schedule.ID,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			result.Failed++
			s.log.ErrorContext(ctx, "encode scheduled insight payload failed",
				slog.String("schedule_id", schedule.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		_, created, err := s.queue.EnqueueIdempotent(ctx, &job.Job{
			UserID:         &schedule.UserID,
			JobType:        job.TypeInsightGenerate,
			Payload:        encoded,
			IdempotencyKey: &idempotencyKey,
			RunAt:          now,
		})
		if err != nil {
			result.Failed++
			s.log.ErrorContext(ctx, "enqueue scheduled insight failed",
				slog.String("schedule_id", schedule.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		if created {
			result.Enqueued++
		} else {
			result.Existing++
		}

		nextRunAt := nextRun(now, location, schedule.Cadence)
		if err := s.repo.AdvanceSchedule(ctx, schedule.ID, schedule.NextRunAt, now, nextRunAt); err != nil {
			s.log.ErrorContext(ctx, "advance scheduled insight failed",
				slog.String("schedule_id", schedule.ID),
				slog.String("error", err.Error()),
			)
		}
	}

	s.log.InfoContext(ctx, "scheduled insight fan-out completed",
		slog.Int("due", result.Due),
		slog.Int("enqueued", result.Enqueued),
		slog.Int("existing", result.Existing),
		slog.Int("failed", result.Failed),
	)
	return result, nil
}

func (s *ScheduleService) budgetBlocked(ctx context.Context, userID string) bool {
	if s.usage == nil {
		return false
	}
	exhausted, err := s.usage.Exhausted(ctx, userID)
	if err != nil {
		s.log.WarnContext(ctx, "scheduled insight budget check failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return false
	}
	return exhausted
}

func scheduleView(schedule *Schedule, timezone string) *ScheduleView {
	view := &ScheduleView{Timezone: timezone}
	if schedule == nil {
		return view
	}

	cadence := schedule.Cadence
	view.Enabled = schedule.Enabled
	view.Cadence = &cadence
	view.LastRunAt = schedule.LastRunAt
	view.UpdatedAt = &schedule.UpdatedAt
	if schedule.Enabled {
		next := schedule.NextRunAt
		view.NextRunAt = &next
	}
	return view
}

func scheduleJobKey(scheduleID, startDate, endDate string) string {
	return uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte("insight-schedule:"+scheduleID+":"+startDate+":"+endDate),
	).String()
}

func nextRun(now time.Time, location *time.Location, cadence string) time.Time {
	local := now.In(location)
	switch cadence {
	case CadenceMonthly:
		candidate := time.Date(local.Year(), local.Month(), 1, 1, 0, 0, 0, location)
		if !candidate.After(local) {
			candidate = candidate.AddDate(0, 1, 0)
		}
		return candidate.UTC()
	default:
		candidate := time.Date(local.Year(), local.Month(), local.Day(), 1, 0, 0, 0, location)
		for candidate.Weekday() != time.Monday || !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate.UTC()
	}
}

func scheduledPeriod(cadence string, runAt time.Time, location *time.Location) (string, string) {
	local := runAt.In(location)
	var start, end time.Time
	switch cadence {
	case CadenceMonthly:
		firstThisMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
		end = firstThisMonth.AddDate(0, 0, -1)
		start = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, location)
	default:
		end = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -1)
		start = end.AddDate(0, 0, -6)
	}
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}
