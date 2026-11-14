package job

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
)

const maxBackoff = 5 * time.Minute

// Worker claims and executes jobs.
type Worker struct {
	queue    *Queue
	handlers map[string]Handler
	workerID string
	poll     time.Duration
	log      *slog.Logger
}

// NewWorker creates a worker.
func NewWorker(queue *Queue, handlers map[string]Handler, workerID string, poll time.Duration, log *slog.Logger) *Worker {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		queue:    queue,
		handlers: handlers,
		workerID: workerID,
		poll:     poll,
		log:      log,
	}
}

// Run processes jobs until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	for {
		processed, err := w.ProcessOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			w.log.Error("job processing failed", slog.String("error", err.Error()))
		}
		if processed {
			continue
		}

		timer := time.NewTimer(w.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// ProcessOnce claims and handles at most one job.
func (w *Worker) ProcessOnce(ctx context.Context) (bool, error) {
	claimed, err := w.queue.Claim(ctx, w.workerID)
	if err != nil {
		return false, err
	}
	if claimed == nil {
		return false, nil
	}

	handler, ok := w.handlers[claimed.JobType]
	if !ok {
		if err := w.queue.Fail(ctx, claimed.ID, "no handler for job type"); err != nil {
			return true, err
		}
		w.log.Error("job has no handler",
			slog.String("job_id", claimed.ID),
			slog.String("job_type", claimed.JobType),
		)
		return true, nil
	}

	if err := handler.Handle(ctx, claimed); err != nil {
		if claimed.Attempts >= claimed.MaxAttempts || !retryable(err) {
			if failErr := w.queue.Fail(ctx, claimed.ID, err.Error()); failErr != nil {
				return true, failErr
			}
			w.log.Error("job failed permanently",
				slog.String("job_id", claimed.ID),
				slog.String("job_type", claimed.JobType),
				slog.Int("attempts", claimed.Attempts),
				slog.String("error", err.Error()),
			)
			return true, nil
		}

		delay := backoff(claimed.Attempts)
		if retryErr := w.queue.Retry(ctx, claimed.ID, err.Error(), time.Now().UTC().Add(delay)); retryErr != nil {
			return true, retryErr
		}
		w.log.Warn("job scheduled for retry",
			slog.String("job_id", claimed.ID),
			slog.String("job_type", claimed.JobType),
			slog.Int("attempts", claimed.Attempts),
			slog.Duration("retry_in", delay),
			slog.String("error", err.Error()),
		)
		return true, nil
	}

	if err := w.queue.Complete(ctx, claimed.ID); err != nil {
		return true, err
	}

	w.log.Info("job completed",
		slog.String("job_id", claimed.ID),
		slog.String("job_type", claimed.JobType),
		slog.Int("attempts", claimed.Attempts),
	)
	return true, nil
}

func retryable(err error) bool {
	if errors.Is(err, ai.ErrInvalidRequest) ||
		errors.Is(err, ai.ErrAuthentication) ||
		errors.Is(err, ai.ErrInvalidResponse) ||
		errors.Is(err, ai.ErrDimensionMismatch) {
		return false
	}
	return true
}

func backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 8 {
		attempts = 8
	}
	delay := time.Duration(1<<uint(attempts-1)) * time.Second
	if delay > maxBackoff {
		return maxBackoff
	}
	return delay
}
