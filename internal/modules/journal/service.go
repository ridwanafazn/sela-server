package journal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
)

const (
	endpointCreateJournal = "POST /journals"
	idempotencyTTL        = 24 * time.Hour
	defaultTimezone       = "Asia/Jakarta"
)

// JobEnqueuer schedules durable work in the same transaction as a journal
// write.
type JobEnqueuer interface {
	EnqueueJournalEmbedding(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash string, runAt time.Time) error
	EnqueueJournalMemoryExtraction(ctx context.Context, tx *gorm.DB, userID, journalID, contentHash string, runAt time.Time) error
}

// Service implements journal rules, optimistic concurrency, and idempotency.
type Service struct {
	repo    *Repository
	jobs    JobEnqueuer
	limiter *ratelimit.Limiter
	limits  Limits
	now     func() time.Time
}

// Limits holds journal write limits.
type Limits struct {
	PerMinute int
}

// NewService creates the journal service. jobs may be nil in tests that do
// not exercise background processing.
func NewService(repo *Repository, jobs JobEnqueuer, limiter *ratelimit.Limiter, limits Limits) *Service {
	return &Service{repo: repo, jobs: jobs, limiter: limiter, limits: limits, now: time.Now}
}

// Create creates a journal or returns the stored response for a retried key.
func (s *Service) Create(ctx context.Context, userID, idempotencyKey string, input CreateInput) (*View, int, error) {
	if err := s.enforceWriteLimit(ctx, userID); err != nil {
		return nil, 0, err
	}
	if err := validateCreate(input); err != nil {
		return nil, 0, err
	}

	entryDate, err := s.resolveEntryDate(ctx, userID, input.EntryDate)
	if err != nil {
		return nil, 0, err
	}

	status := StatusActive
	if input.Status != nil {
		status = *input.Status
	}

	requestHash := createRequestHash(input)
	now := s.now().UTC()

	journalID, err := ids.New()
	if err != nil {
		return nil, 0, err
	}
	recordID, err := ids.New()
	if err != nil {
		return nil, 0, err
	}

	journal := &Journal{
		ID:        journalID,
		UserID:    userID,
		Title:     input.Title,
		Content:   input.Content,
		EntryDate: entryDate,
		Status:    status,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	record := &IdempotencyRecord{
		ID:          recordID,
		UserID:      userID,
		Key:         idempotencyKey,
		Endpoint:    endpointCreateJournal,
		RequestHash: requestHash,
		ExpiresAt:   now.Add(idempotencyTTL),
		CreatedAt:   now,
	}

	var view *View

	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		claimed, err := s.repo.ClaimIdempotency(tx, record)
		if err != nil {
			return err
		}

		if !claimed {
			existing, err := s.repo.IdempotencyRecord(tx, userID, endpointCreateJournal, idempotencyKey)
			if err != nil {
				return err
			}
			if existing.RequestHash != requestHash {
				return apierrors.IdempotencyKeyReused()
			}
			if !existing.ExpiresAt.After(now) {
				return apierrors.Conflict("The idempotency key has expired. Use a new key.")
			}
			if existing.ResponseStatus == nil || existing.ResponseBody == nil {
				return apierrors.Conflict("The request is still being processed.")
			}

			var replay View
			if err := json.Unmarshal(existing.ResponseBody, &replay); err != nil {
				return err
			}
			view = &replay
			return nil
		}

		if err := s.repo.Create(tx, journal); err != nil {
			return err
		}
		if journal.Status == StatusActive && s.jobs != nil {
			hash := ContentHash(journal.Content)
			if err := s.jobs.EnqueueJournalEmbedding(ctx, tx, userID, journal.ID, hash, now); err != nil {
				return err
			}
			if err := s.jobs.EnqueueJournalMemoryExtraction(ctx, tx, userID, journal.ID, hash, now); err != nil {
				return err
			}
		}

		created := newView(journal)
		body, err := json.Marshal(created)
		if err != nil {
			return err
		}
		if err := s.repo.CompleteIdempotency(tx, record.ID, http.StatusCreated, body); err != nil {
			return err
		}

		view = &created
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return view, http.StatusCreated, nil
}

// List returns a keyset-paginated journal page.
func (s *Service) List(ctx context.Context, userID string, filter ListFilter) (*Page, error) {
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	var (
		afterDate *time.Time
		afterID   string
	)

	if filter.Cursor != "" {
		entryDate, id, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "cursor",
				Code:    "INVALID",
				Message: "The cursor is invalid.",
			})
		}
		afterDate = &entryDate
		afterID = id
	}

	journals, err := s.repo.List(ctx, userID, filter, afterDate, afterID)
	if err != nil {
		return nil, err
	}

	hasMore := len(journals) > filter.Limit
	if hasMore {
		journals = journals[:filter.Limit]
	}

	items := make([]View, 0, len(journals))
	for index := range journals {
		items = append(items, newView(&journals[index]))
	}

	var nextCursor *string
	if hasMore && len(journals) > 0 {
		last := journals[len(journals)-1]
		cursor := encodeCursor(last.EntryDate, last.ID)
		nextCursor = &cursor
	}

	return &Page{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// Get returns one owned journal.
func (s *Service) Get(ctx context.Context, userID, journalID string) (*View, error) {
	entry, err := s.repo.Get(ctx, userID, journalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apierrors.NotFound("The journal was not found.")
	}
	if err != nil {
		return nil, err
	}

	view := newView(entry)
	return &view, nil
}

// Update applies an optimistic-concurrency update.
func (s *Service) Update(ctx context.Context, userID, journalID string, ifMatch int, input UpdateInput) (*View, error) {
	if err := s.enforceWriteLimit(ctx, userID); err != nil {
		return nil, err
	}
	if err := validateUpdate(input); err != nil {
		return nil, err
	}

	var view *View

	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		entry, err := s.repo.GetForUpdate(tx, userID, journalID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.NotFound("The journal was not found.")
		}
		if err != nil {
			return err
		}
		if entry.Version != ifMatch {
			return apierrors.VersionConflict("The journal was modified by another request.")
		}
		if input.IsEmpty() {
			current := newView(entry)
			view = &current
			return nil
		}

		previousContent := entry.Content
		previousStatus := entry.Status

		if input.Title.Set {
			entry.Title = input.Title.Value
		}
		if input.Content.Set {
			entry.Content = *input.Content.Value
		}
		if input.EntryDate.Set {
			entryDate, _ := time.Parse("2006-01-02", *input.EntryDate.Value)
			entry.EntryDate = entryDate
		}
		if input.Status.Set {
			entry.Status = *input.Status.Value
		}

		entry.Version++
		entry.UpdatedAt = s.now().UTC()

		if err := s.repo.SaveTx(tx, entry); err != nil {
			return err
		}
		if previousStatus != entry.Status {
			if err := memory.ReconcileJournalSourcesTx(ctx, tx, userID, entry.ID, entry.UpdatedAt); err != nil {
				return err
			}
		}
		if entry.Status == StatusActive && s.jobs != nil &&
			(entry.Content != previousContent || previousStatus != StatusActive) {
			hash := ContentHash(entry.Content)
			if err := s.jobs.EnqueueJournalEmbedding(ctx, tx, userID, entry.ID, hash, entry.UpdatedAt); err != nil {
				return err
			}
			if err := s.jobs.EnqueueJournalMemoryExtraction(ctx, tx, userID, entry.ID, hash, entry.UpdatedAt); err != nil {
				return err
			}
		}

		updated := newView(entry)
		view = &updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	return view, nil
}

// Delete soft-deletes a journal and advances its version.
func (s *Service) Delete(ctx context.Context, userID, journalID string, ifMatch int) error {
	if err := s.enforceWriteLimit(ctx, userID); err != nil {
		return err
	}
	return s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		entry, err := s.repo.GetForUpdate(tx, userID, journalID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.NotFound("The journal was not found.")
		}
		if err != nil {
			return err
		}
		if entry.Version != ifMatch {
			return apierrors.VersionConflict("The journal was modified by another request.")
		}

		if err := s.repo.SoftDeleteTx(tx, journalID, userID, entry.Version+1, s.now().UTC()); err != nil {
			return err
		}
		now := s.now().UTC()
		if err := memory.ReconcileJournalSourcesTx(ctx, tx, userID, journalID, now); err != nil {
			return err
		}
		return s.repo.DeleteEmbeddingsTx(tx, journalID)
	})
}

func (s *Service) resolveEntryDate(ctx context.Context, userID string, explicit *string) (time.Time, error) {
	if explicit != nil {
		return time.Parse("2006-01-02", *explicit)
	}

	timezone, err := s.repo.ProfileTimezone(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		location, err = time.LoadLocation(defaultTimezone)
		if err != nil {
			return time.Time{}, err
		}
	}

	local := s.now().In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), nil
}

func validateCreate(input CreateInput) error {
	var details []apierrors.Detail

	if input.Title != nil && utf8.RuneCountInString(*input.Title) > 200 {
		details = append(details, apierrors.Detail{
			Field:   "title",
			Code:    "TOO_LONG",
			Message: "Title must be at most 200 characters.",
		})
	}
	if len(input.Content) == 0 {
		details = append(details, apierrors.Detail{
			Field:   "content",
			Code:    "REQUIRED",
			Message: "Content must not be empty.",
		})
	}
	if input.EntryDate != nil {
		if _, err := time.Parse("2006-01-02", *input.EntryDate); err != nil {
			details = append(details, apierrors.Detail{
				Field:   "entry_date",
				Code:    "INVALID_FORMAT",
				Message: "Entry date must use YYYY-MM-DD.",
			})
		}
	}
	if input.Status != nil && !validStatus(*input.Status) {
		details = append(details, apierrors.Detail{
			Field:   "status",
			Code:    "INVALID_VALUE",
			Message: "Status must be draft or active.",
		})
	}

	if len(details) > 0 {
		return apierrors.Validation("The request is invalid.", details...)
	}
	return nil
}

func validateUpdate(input UpdateInput) error {
	var details []apierrors.Detail

	if input.Title.Set && input.Title.Value != nil && utf8.RuneCountInString(*input.Title.Value) > 200 {
		details = append(details, apierrors.Detail{
			Field:   "title",
			Code:    "TOO_LONG",
			Message: "Title must be at most 200 characters.",
		})
	}
	if input.Content.Set {
		if input.Content.Value == nil {
			details = append(details, apierrors.Detail{
				Field:   "content",
				Code:    "INVALID_VALUE",
				Message: "Content must not be null.",
			})
		} else if len(*input.Content.Value) == 0 {
			details = append(details, apierrors.Detail{
				Field:   "content",
				Code:    "REQUIRED",
				Message: "Content must not be empty.",
			})
		}
	}
	if input.EntryDate.Set {
		if input.EntryDate.Value == nil {
			details = append(details, apierrors.Detail{
				Field:   "entry_date",
				Code:    "INVALID_VALUE",
				Message: "Entry date must not be null.",
			})
		} else if _, err := time.Parse("2006-01-02", *input.EntryDate.Value); err != nil {
			details = append(details, apierrors.Detail{
				Field:   "entry_date",
				Code:    "INVALID_FORMAT",
				Message: "Entry date must use YYYY-MM-DD.",
			})
		}
	}
	if input.Status.Set {
		if input.Status.Value == nil || !validStatus(*input.Status.Value) {
			details = append(details, apierrors.Detail{
				Field:   "status",
				Code:    "INVALID_VALUE",
				Message: "Status must be draft or active.",
			})
		}
	}

	if len(details) > 0 {
		return apierrors.Validation("The request is invalid.", details...)
	}
	return nil
}

func validStatus(status string) bool {
	return status == StatusDraft || status == StatusActive
}

func (s *Service) enforceWriteLimit(ctx context.Context, userID string) error {
	if s.limiter == nil {
		return nil
	}
	decision, err := s.limiter.Check(ctx, "journal.write", userID, s.limits.PerMinute, time.Minute)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apierrors.RateLimited(decision.RetryAfter)
	}
	return s.limiter.Record(ctx, "journal.write", userID)
}

// ContentHash returns the canonical content hash used for derived-work job
// idempotency.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func createRequestHash(input CreateInput) string {
	payload, _ := json.Marshal(struct {
		Title     *string `json:"title"`
		Content   string  `json:"content"`
		EntryDate *string `json:"entry_date"`
		Status    *string `json:"status"`
	}{
		Title:     input.Title,
		Content:   input.Content,
		EntryDate: input.EntryDate,
		Status:    input.Status,
	})

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
