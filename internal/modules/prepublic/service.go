package prepublic

import (
	"context"
	"log/slog"
	"time"

	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

const exportRateKey = "prepublic.export"

// Limits holds export limits.
type Limits struct {
	ExportPerHour int
}

// Service applies consent, entitlement, and export rules.
type Service struct {
	repo           *Repository
	limiter        *ratelimit.Limiter
	limits         Limits
	termsVersion   string
	privacyVersion string
	log            *slog.Logger
	now            func() time.Time
}

// NewService creates the pre-public service.
func NewService(
	repo *Repository,
	limiter *ratelimit.Limiter,
	limits Limits,
	termsVersion, privacyVersion string,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:           repo,
		limiter:        limiter,
		limits:         limits,
		termsVersion:   termsVersion,
		privacyVersion: privacyVersion,
		log:            log,
		now:            time.Now,
	}
}

// GetConsents returns the current account gate state.
func (s *Service) GetConsents(ctx context.Context, userID string) (*ConsentSummary, error) {
	return s.repo.GetConsents(ctx, userID)
}

// UpdateConsents records adult confirmation and legal acceptance.
func (s *Service) UpdateConsents(ctx context.Context, userID string, input ConsentInput) (*ConsentSummary, error) {
	var details []apierrors.Detail

	if input.AdultConfirmed != nil && !*input.AdultConfirmed {
		details = append(details, apierrors.Detail{
			Field:   "adult_confirmed",
			Code:    "REQUIRED",
			Message: "Adult confirmation is required to use SELA.",
		})
	}
	if input.TermsVersion != nil && *input.TermsVersion != s.termsVersion {
		details = append(details, apierrors.Detail{
			Field:   "terms_version",
			Code:    "OUTDATED",
			Message: "Accept the current terms version.",
		})
	}
	if input.PrivacyVersion != nil && *input.PrivacyVersion != s.privacyVersion {
		details = append(details, apierrors.Detail{
			Field:   "privacy_version",
			Code:    "OUTDATED",
			Message: "Accept the current privacy version.",
		})
	}
	if len(details) > 0 {
		return nil, apierrors.Validation("The request is invalid.", details...)
	}

	adult := input.AdultConfirmed != nil && *input.AdultConfirmed
	now := s.now().UTC()
	if err := s.repo.UpdateConsents(ctx, userID, adult, input.TermsVersion, input.PrivacyVersion, now); err != nil {
		return nil, err
	}
	return s.GetConsents(ctx, userID)
}

// GetEntitlement returns the provider-independent plan state.
func (s *Service) GetEntitlement(ctx context.Context, userID string) (*Entitlement, error) {
	return s.repo.GetEntitlement(ctx, userID)
}

// Export returns the user's data after applying the export rate limit.
func (s *Service) Export(ctx context.Context, userID string) (map[string]any, error) {
	if s.limiter != nil && s.limits.ExportPerHour > 0 {
		decision, err := s.limiter.Check(ctx, exportRateKey, userID, s.limits.ExportPerHour, time.Hour)
		if err != nil {
			return nil, err
		}
		if !decision.Allowed {
			return nil, apierrors.RateLimited(decision.RetryAfter)
		}
		if err := s.limiter.Record(ctx, exportRateKey, userID); err != nil {
			return nil, err
		}
	}

	data, err := s.repo.Export(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "user data exported", slog.String("user_id", userID))
	return data, nil
}
