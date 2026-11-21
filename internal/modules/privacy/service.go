package privacy

import (
	"context"
	"log/slog"
	"time"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

// Service reads and updates privacy preferences.
type Service struct {
	repo *Repository
	log  *slog.Logger
	now  func() time.Time
}

// NewService creates the privacy service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, log: log, now: time.Now}
}

// Get returns preferences and relevant consent state.
func (s *Service) Get(ctx context.Context, userID string) (*View, error) {
	preferences, err := s.repo.Get(ctx, userID, s.now().UTC())
	if err != nil {
		return nil, err
	}
	consents, err := s.repo.Consents(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &View{
		AIContextMode:           preferences.AIContextMode,
		MemoryExtractionEnabled: preferences.MemoryExtractionEnabled,
		SensitiveMemoryGranted:  consents.SensitiveMemoryGranted,
		AdultConfirmed:          consents.AdultConfirmed,
		TermsVersion:            consents.TermsVersion,
		PrivacyVersion:          consents.PrivacyVersion,
		UpdatedAt:               preferences.UpdatedAt,
	}, nil
}

// Update applies a partial update.
func (s *Service) Update(ctx context.Context, userID string, input UpdateInput) (*View, error) {
	if input.AIContextMode != nil {
		switch *input.AIContextMode {
		case ContextNone, ContextJournals, ContextJournalsAndMemories:
		default:
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "ai_context_mode",
				Code:    "INVALID_VALUE",
				Message: "AI context mode must be none, journals, or journals_and_memories.",
			})
		}
	}

	if _, err := s.repo.Update(ctx, userID, input.AIContextMode, input.MemoryExtractionEnabled, s.now().UTC()); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID)
}

// AIContextMode returns the effective personal-context policy.
func (s *Service) AIContextMode(ctx context.Context, userID string) (string, error) {
	preferences, err := s.repo.Current(ctx, userID)
	if err != nil {
		return ContextNone, err
	}
	return preferences.AIContextMode, nil
}

// MemoryExtractionEnabled reports whether automatic memory extraction is on.
func (s *Service) MemoryExtractionEnabled(ctx context.Context, userID string) (bool, error) {
	preferences, err := s.repo.Current(ctx, userID)
	if err != nil {
		return false, err
	}
	return preferences.MemoryExtractionEnabled, nil
}
