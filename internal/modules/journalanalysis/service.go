package journalanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

const (
	retryAfterSeconds = 2
	analysisHourKey   = "journal.analysis.hour"
	analysisDayKey    = "journal.analysis.day"
	statusQueued      = "queued"
	statusProcessing  = "processing"
	statusCompleted   = "completed"
	statusFailed      = "failed"
)

// JournalReader loads one owned journal.
type JournalReader interface {
	Get(ctx context.Context, userID, journalID string) (*journal.Journal, error)
}

// Service implements analysis requests, inspection, deletion, and job status.
type Service struct {
	repo     *Repository
	journals JournalReader
	queue    *job.Queue
	limiter  *ratelimit.Limiter
	usage    *aiusage.Service
	limits   Limits
	log      *slog.Logger
	now      func() time.Time
}

// NewService creates the journal analysis service.
func NewService(
	repo *Repository,
	journals JournalReader,
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
		repo:     repo,
		journals: journals,
		queue:    queue,
		limiter:  limiter,
		usage:    usage,
		limits:   limits,
		log:      log,
		now:      time.Now,
	}
}

// Request queues an asynchronous analysis for one owned, published journal.
func (s *Service) Request(
	ctx context.Context,
	userID, journalID, idempotencyKey string,
) (*AcceptedView, error) {
	entry, err := s.journals.Get(ctx, userID, journalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, journalNotFound()
	}
	if err != nil {
		return nil, err
	}
	if entry.Status != journal.StatusActive {
		return nil, apierrors.Conflict("Publish the journal before requesting an analysis.")
	}

	existing, err := s.queue.FindByUserAndKey(ctx, userID, idempotencyKey)
	if err == nil {
		if !sameRequest(existing, journalID, entry.Version) {
			return nil, apierrors.IdempotencyKeyReused()
		}
		return acceptedView(existing.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if _, err := s.repo.GetByJournalVersion(ctx, userID, journalID, entry.Version); err == nil {
		return nil, apierrors.Conflict("An analysis already exists for this journal version. Delete it before requesting another.")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	activeJobID, err := s.repo.FindActiveJob(ctx, userID, journalID, entry.Version)
	if err == nil {
		return acceptedView(activeJobID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := s.enforceRateLimit(ctx, userID); err != nil {
		return nil, err
	}
	if s.usage != nil {
		exhausted, err := s.usage.Exhausted(ctx, userID)
		if err != nil {
			s.log.WarnContext(ctx, "journal analysis budget check failed", slog.String("error", err.Error()))
		} else if exhausted {
			return nil, apierrors.RateLimited(24 * time.Hour)
		}
	}

	payload, err := json.Marshal(JobPayload{
		JournalID:      journalID,
		JournalVersion: entry.Version,
	})
	if err != nil {
		return nil, err
	}

	jobID, created, err := s.queue.EnqueueIdempotent(ctx, &job.Job{
		UserID:         &userID,
		JobType:        job.TypeJournalAnalysis,
		Payload:        payload,
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
		if !sameRequest(existing, journalID, entry.Version) {
			return nil, apierrors.IdempotencyKeyReused()
		}
		jobID = existing.ID
	}

	return acceptedView(jobID), nil
}

// Get returns the newest analysis for one owned journal.
func (s *Service) Get(ctx context.Context, userID, journalID string) (*View, error) {
	row, err := s.repo.GetLatest(ctx, userID, journalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, analysisNotFound()
	}
	if err != nil {
		return nil, err
	}
	return newView(row), nil
}

// Delete removes every stored analysis for one owned journal and cancels
// pending jobs.
func (s *Service) Delete(ctx context.Context, userID, journalID string) error {
	found, err := s.repo.DeleteForJournal(ctx, userID, journalID, s.now().UTC())
	if err != nil {
		return err
	}
	if !found {
		return analysisNotFound()
	}
	return nil
}

// JobStatus returns one owned analysis job.
func (s *Service) JobStatus(ctx context.Context, userID, jobID string) (*JobView, error) {
	claimed, err := s.queue.Get(ctx, userID, jobID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, analysisNotFound()
	}
	if err != nil {
		return nil, err
	}
	if claimed.JobType != job.TypeJournalAnalysis {
		return nil, analysisNotFound()
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
			view.AnalysisID = result.AnalysisID
			if result.Message != "" {
				view.Message = &result.Message
			}
		}
	case job.StatusFailed:
		view.Status = statusFailed
		view.RetryAfterSeconds = 1
		view.Error = &JobError{
			Code:      "JOURNAL_ANALYSIS_FAILED",
			Message:   "Journal analysis failed. Try again with a new request.",
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

	hour, err := s.limiter.Check(ctx, analysisHourKey, userID, s.limits.PerHour, time.Hour)
	if err != nil {
		return err
	}
	if !hour.Allowed {
		return apierrors.RateLimited(hour.RetryAfter)
	}
	day, err := s.limiter.Check(ctx, analysisDayKey, userID, s.limits.PerDay, 24*time.Hour)
	if err != nil {
		return err
	}
	if !day.Allowed {
		return apierrors.RateLimited(day.RetryAfter)
	}
	if err := s.limiter.Record(ctx, analysisHourKey, userID); err != nil {
		return err
	}
	return s.limiter.Record(ctx, analysisDayKey, userID)
}

func sameRequest(existing *job.Job, journalID string, version int) bool {
	if existing.JobType != job.TypeJournalAnalysis {
		return false
	}
	var payload JobPayload
	if err := json.Unmarshal(existing.Payload, &payload); err != nil {
		return false
	}
	return payload.JournalID == journalID && payload.JournalVersion == version
}

func acceptedView(jobID string) *AcceptedView {
	return &AcceptedView{
		JobID:             jobID,
		Status:            statusQueued,
		StatusURL:         "/api/v1/journal-analysis-jobs/" + jobID,
		RetryAfterSeconds: retryAfterSeconds,
	}
}

func analysisNotFound() *apierrors.Error {
	return apierrors.NotFound("The journal analysis was not found.")
}

func journalNotFound() *apierrors.Error {
	return apierrors.NotFound("The journal was not found.")
}
