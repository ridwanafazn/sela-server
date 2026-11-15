package insight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

const (
	defaultPageLimit  = 20
	maxPageLimit      = 100
	retryAfterSeconds = 2
	maxInsightFocus   = 500
	maxFeedbackNote   = 2000
	insightHourKey    = "insight.generate.hour"
	insightDayKey     = "insight.generate.day"
	maxDateRangeDays  = 366
	statusQueued      = "queued"
	statusProcessing  = "processing"
	statusCompleted   = "completed"
	statusFailed      = "failed"
)

// Service implements insight requests, review, and job status.
type Service struct {
	repo    *Repository
	queue   *job.Queue
	limiter *ratelimit.Limiter
	usage   *aiusage.Service
	limits  Limits
	log     *slog.Logger
	now     func() time.Time
}

// NewService creates the insight service.
func NewService(
	repo *Repository,
	queue *job.Queue,
	limiter *ratelimit.Limiter,
	usage *aiusage.Service,
	limits Limits,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:    repo,
		queue:   queue,
		limiter: limiter,
		usage:   usage,
		limits:  limits,
		log:     log,
		now:     time.Now,
	}
}

// Request queues an asynchronous insight generation job.
func (s *Service) Request(
	ctx context.Context,
	userID, idempotencyKey string,
	input GenerateInput,
) (*AcceptedView, error) {
	if err := validateGenerateInput(input); err != nil {
		return nil, err
	}
	focus := strings.TrimSpace(input.Focus)
	hash := requestHash(input.Period, input.StartDate, input.EndDate, focus)

	existing, err := s.queue.FindByUserAndKey(ctx, userID, idempotencyKey)
	if err == nil {
		if !sameRequest(existing.Payload, hash) {
			return nil, apierrors.IdempotencyKeyReused()
		}
		return acceptedView(existing.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	_, timezone, err := s.repo.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.FixedZone("UTC", 0)
	}

	period, err := resolvePeriod(input, location, s.now().UTC())
	if err != nil {
		return nil, err
	}

	payload := JobPayload{
		Period:         input.Period,
		StartDate:      period.start.Format("2006-01-02"),
		EndDate:        period.end.Format("2006-01-02"),
		Focus:          focus,
		GenerationType: period.generationType,
		RequestHash:    hash,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if err := s.enforceRateLimit(ctx, userID); err != nil {
		return nil, err
	}
	if s.usage != nil {
		exhausted, err := s.usage.Exhausted(ctx, userID)
		if err != nil {
			s.log.WarnContext(ctx, "insight budget check failed", slog.String("error", err.Error()))
		} else if exhausted {
			s.log.WarnContext(ctx, "insight request blocked by ai budget", slog.String("user_id", userID))
			return nil, apierrors.RateLimited(24 * time.Hour)
		}
	}

	jobID, created, err := s.queue.EnqueueIdempotent(ctx, &job.Job{
		UserID:         &userID,
		JobType:        job.TypeInsightGenerate,
		Payload:        encoded,
		IdempotencyKey: &idempotencyKey,
		RunAt:          s.now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	if !created {
		existing, err := s.queue.FindByUserAndKey(ctx, userID, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if !sameRequest(existing.Payload, payload.RequestHash) {
			return nil, apierrors.IdempotencyKeyReused()
		}
		jobID = existing.ID
	}

	return acceptedView(jobID), nil
}

// List returns a page of completed insights.
func (s *Service) List(ctx context.Context, userID string, filter ListFilter) (*Page, error) {
	switch filter.Period {
	case "", "all", GenerationWeekly, GenerationMonthly:
	default:
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "period",
			Code:    "INVALID_VALUE",
			Message: "Period must be all, weekly, or monthly.",
		})
	}

	limit := clampLimit(filter.Limit)
	var (
		afterCreatedAt *time.Time
		afterID        string
	)
	if filter.Cursor != "" {
		createdAt, id, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "cursor",
				Code:    "INVALID",
				Message: "The cursor is invalid.",
			})
		}
		afterCreatedAt = &createdAt
		afterID = id
	}

	items, err := s.repo.List(ctx, userID, filter.Period, afterCreatedAt, afterID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor *string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		value := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &value
	}
	return &Page{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// Get returns one owned insight with evidence and latest feedback.
func (s *Service) Get(ctx context.Context, userID, insightID string) (*Detail, error) {
	record, err := s.repo.Get(ctx, userID, insightID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, insightNotFound()
	}
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.Evidence(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	feedback, err := s.repo.LatestFeedback(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	return &Detail{View: newView(record, evidence, feedback)}, nil
}

// Delete soft-deletes an insight.
func (s *Service) Delete(ctx context.Context, userID, insightID string) error {
	deleted, err := s.repo.SoftDelete(ctx, userID, insightID, s.now().UTC())
	if err != nil {
		return err
	}
	if !deleted {
		return insightNotFound()
	}
	return nil
}

// Feedback appends an assessment to an insight.
func (s *Service) Feedback(ctx context.Context, userID, insightID string, input FeedbackInput) (*Detail, error) {
	var note *string
	switch input.Action {
	case "confirm", "reject":
	case "correct":
		if input.Note == nil || strings.TrimSpace(*input.Note) == "" {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "note",
				Code:    "REQUIRED",
				Message: "A note is required when action is correct.",
			})
		}
	default:
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "action",
			Code:    "INVALID_VALUE",
			Message: "Action must be confirm, correct, or reject.",
		})
	}

	if input.Note != nil {
		trimmed := strings.TrimSpace(*input.Note)
		if utf8.RuneCountInString(trimmed) > maxFeedbackNote {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "note",
				Code:    "TOO_LONG",
				Message: "Note must be at most 2000 characters.",
			})
		}
		note = &trimmed
	}

	record, err := s.repo.Get(ctx, userID, insightID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, insightNotFound()
	}
	if err != nil {
		return nil, err
	}

	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if err := s.repo.AddFeedback(ctx, &Feedback{
		ID:        id,
		InsightID: record.ID,
		UserID:    userID,
		Action:    input.Action,
		Note:      note,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, insightID)
}

// JobStatus returns one owned insight generation job.
func (s *Service) JobStatus(ctx context.Context, userID, jobID string) (*JobView, error) {
	claimed, err := s.queue.Get(ctx, userID, jobID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, insightNotFound()
	}
	if err != nil {
		return nil, err
	}
	if claimed.JobType != job.TypeInsightGenerate {
		return nil, insightNotFound()
	}

	view := &JobView{
		JobID:             claimed.ID,
		RetryAfterSeconds: retryAfterSeconds,
		CreatedAt:         claimed.CreatedAt,
		UpdatedAt:         claimed.UpdatedAt,
	}

	switch claimed.Status {
	case job.StatusPending, job.StatusRetry:
		view.Status = statusQueued
	case job.StatusProcessing:
		view.Status = statusProcessing
	case job.StatusCompleted:
		view.Status = statusCompleted
		view.RetryAfterSeconds = 1
		var result GenerationResult
		if len(claimed.Result) > 0 {
			if err := json.Unmarshal(claimed.Result, &result); err != nil {
				return nil, err
			}
			if result.Outcome != "" {
				view.Outcome = &result.Outcome
			}
			view.InsightID = result.InsightID
			if result.Message != "" {
				view.Message = &result.Message
			}
		}
	case job.StatusFailed:
		view.Status = statusFailed
		view.RetryAfterSeconds = 1
		view.Error = &JobError{
			Code:      "INSIGHT_GENERATION_FAILED",
			Message:   "Insight generation failed. Try again with a new request.",
			Retryable: false,
		}
	default:
		view.Status = statusQueued
	}
	return view, nil
}

func (s *Service) enforceRateLimit(ctx context.Context, userID string) error {
	if s.limiter == nil {
		return nil
	}

	hour, err := s.limiter.Check(ctx, insightHourKey, userID, s.limits.PerHour, time.Hour)
	if err != nil {
		return err
	}
	if !hour.Allowed {
		return apierrors.RateLimited(hour.RetryAfter)
	}
	day, err := s.limiter.Check(ctx, insightDayKey, userID, s.limits.PerDay, 24*time.Hour)
	if err != nil {
		return err
	}
	if !day.Allowed {
		return apierrors.RateLimited(day.RetryAfter)
	}
	if err := s.limiter.Record(ctx, insightHourKey, userID); err != nil {
		return err
	}
	return s.limiter.Record(ctx, insightDayKey, userID)
}

type periodDates struct {
	start          time.Time
	end            time.Time
	generationType string
}

func resolvePeriod(input GenerateInput, location *time.Location, now time.Time) (periodDates, error) {
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)

	var start, end time.Time
	switch input.Period {
	case GenerationWeekly:
		start = today.AddDate(0, 0, -6)
		end = today
	case GenerationMonthly:
		start = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = today
	case "custom":
	default:
		return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "period",
			Code:    "INVALID_VALUE",
			Message: "Period must be weekly, monthly, or custom.",
		})
	}

	if input.StartDate != nil || input.EndDate != nil {
		if input.StartDate == nil || input.EndDate == nil {
			return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "start_date",
				Code:    "REQUIRED",
				Message: "Start date and end date must be provided together.",
			})
		}
		parsedStart, err := time.Parse("2006-01-02", *input.StartDate)
		if err != nil {
			return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "start_date",
				Code:    "INVALID_FORMAT",
				Message: "Start date must use YYYY-MM-DD.",
			})
		}
		parsedEnd, err := time.Parse("2006-01-02", *input.EndDate)
		if err != nil {
			return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "end_date",
				Code:    "INVALID_FORMAT",
				Message: "End date must use YYYY-MM-DD.",
			})
		}
		start = parsedStart
		end = parsedEnd
	}

	if input.Period == "custom" && (input.StartDate == nil || input.EndDate == nil) {
		return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "start_date",
			Code:    "REQUIRED",
			Message: "Start date and end date are required for a custom period.",
		})
	}
	if start.After(end) {
		return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "start_date",
			Code:    "INVALID_RANGE",
			Message: "Start date must not be after the end date.",
		})
	}
	if end.Sub(start) > maxDateRangeDays*24*time.Hour {
		return periodDates{}, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "end_date",
			Code:    "INVALID_RANGE",
			Message: "The requested period must be at most 366 days.",
		})
	}

	generationType := GenerationOnDemand
	switch input.Period {
	case GenerationWeekly:
		generationType = GenerationWeekly
	case GenerationMonthly:
		generationType = GenerationMonthly
	}
	return periodDates{start: start, end: end, generationType: generationType}, nil
}

func validateGenerateInput(input GenerateInput) error {
	switch input.Period {
	case GenerationWeekly, GenerationMonthly, "custom":
	default:
		return apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "period",
			Code:    "INVALID_VALUE",
			Message: "Period must be weekly, monthly, or custom.",
		})
	}
	if utf8.RuneCountInString(strings.TrimSpace(input.Focus)) > maxInsightFocus {
		return apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "focus",
			Code:    "TOO_LONG",
			Message: "Focus must be at most 500 characters.",
		})
	}
	return nil
}

func requestHash(period string, startDate, endDate *string, focus string) string {
	encoded, _ := json.Marshal(struct {
		Period    string  `json:"period"`
		StartDate *string `json:"start_date"`
		EndDate   *string `json:"end_date"`
		Focus     string  `json:"focus"`
	}{
		Period:    period,
		StartDate: startDate,
		EndDate:   endDate,
		Focus:     focus,
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func sameRequest(payload []byte, want string) bool {
	var stored JobPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return false
	}
	return stored.RequestHash == want
}

func acceptedView(jobID string) *AcceptedView {
	return &AcceptedView{
		JobID:             jobID,
		Status:            statusQueued,
		StatusURL:         "/api/v1/insight-jobs/" + jobID,
		RetryAfterSeconds: retryAfterSeconds,
	}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > maxPageLimit {
		return maxPageLimit
	}
	return limit
}

func insightNotFound() *apierrors.Error {
	return apierrors.NotFound("The insight was not found.")
}
