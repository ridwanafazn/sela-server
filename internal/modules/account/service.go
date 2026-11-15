package account

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

const (
	maxPasswordLength = 1024
	deletionMessage   = "Your account has been deactivated. Data deletion is in progress."
	rateLimitKey      = "account.delete"
)

// Limits holds account deletion re-authentication limits.
type Limits struct {
	PerHour int
}

// PasswordVerifier verifies an Argon2id password hash.
type PasswordVerifier interface {
	Verify(password, encoded string) bool
}

// JobEnqueuer schedules the durable purge job.
type JobEnqueuer interface {
	EnqueueAccountDeletion(ctx context.Context, tx *gorm.DB, userID string, runAt time.Time) error
}

// Service implements account deletion rules.
type Service struct {
	repo      *Repository
	passwords PasswordVerifier
	jobs      JobEnqueuer
	limiter   *ratelimit.Limiter
	limits    Limits
	log       *slog.Logger
	now       func() time.Time
}

// NewService creates the account service.
func NewService(
	repo *Repository,
	passwords PasswordVerifier,
	jobs JobEnqueuer,
	limiter *ratelimit.Limiter,
	limits Limits,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:      repo,
		passwords: passwords,
		jobs:      jobs,
		limiter:   limiter,
		limits:    limits,
		log:       log,
		now:       time.Now,
	}
}

// DeleteAccount deactivates the account after re-authentication and schedules
// the asynchronous purge.
func (s *Service) DeleteAccount(
	ctx context.Context,
	userID, password, confirmation string,
) (*DeletionView, error) {
	if confirmation != "DELETE" {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "confirmation",
			Code:    "INVALID_VALUE",
			Message: "Confirmation must be DELETE.",
		})
	}
	if strings.TrimSpace(password) == "" {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "password",
			Code:    "REQUIRED",
			Message: "Password is required.",
		})
	}
	if utf8.RuneCountInString(password) > maxPasswordLength {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "password",
			Code:    "TOO_LONG",
			Message: "Password is too long.",
		})
	}
	if s.jobs == nil {
		return nil, apierrors.Internal()
	}
	if err := s.enforceRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	account, err := s.repo.Account(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apierrors.Unauthenticated("Authentication is required.")
	}
	if err != nil {
		return nil, err
	}
	if account.Status != "active" || account.DeletedAt != nil {
		return nil, apierrors.Conflict("The account is already scheduled for deletion.")
	}
	if !s.passwords.Verify(password, account.PasswordHash) {
		return nil, apierrors.Unauthenticated("The password is incorrect.")
	}

	now := s.now().UTC()
	changed, err := s.repo.BeginDeletion(ctx, userID, now, func(tx *gorm.DB) error {
		return s.jobs.EnqueueAccountDeletion(ctx, tx, userID, now)
	})
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, apierrors.Conflict("The account is already scheduled for deletion.")
	}

	s.log.InfoContext(ctx, "account deletion requested", slog.String("user_id", userID))

	return &DeletionView{
		Status:           "pending",
		PurgeScheduledAt: now,
		Message:          deletionMessage,
	}, nil
}

func (s *Service) enforceRateLimit(ctx context.Context, userID string) error {
	if s.limiter == nil {
		return nil
	}
	decision, err := s.limiter.Check(ctx, rateLimitKey, userID, s.limits.PerHour, time.Hour)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apierrors.RateLimited(decision.RetryAfter)
	}
	return s.limiter.Record(ctx, rateLimitKey, userID)
}
