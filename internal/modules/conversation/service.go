package conversation

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

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
	"github.com/ridwanafazn/sela-server/internal/shared/ids"
	"github.com/ridwanafazn/sela-server/prompts"
)

const (
	idempotencyTTL        = 24 * time.Hour
	pendingIdempotencyAge = 2 * time.Minute
	historyTokenBudget    = 1500
	maxOutputTokens       = 800
	conversationPageLimit = 100
	defaultPageLimit      = 20
	companionRateLimitKey = "companion.message"
)

// Limits holds Companion rate limits.
type Limits struct {
	CompanionPerMinute int
}

// Service implements conversation persistence and the Companion pipeline.
type Service struct {
	repo      *Repository
	retrieval *journal.RetrievalService
	memories  MemoryRetriever
	jobs      JobEnqueuer
	llm       ai.LLMProvider
	limiter   *ratelimit.Limiter
	limits    Limits
	model     string
	log       *slog.Logger
	now       func() time.Time
}

// MemoryRetriever returns confirmed and candidate memories relevant to a
// message.
type MemoryRetriever interface {
	Retrieve(ctx context.Context, userID, query string, limit int) ([]memory.RetrievedMemory, error)
}

// JobEnqueuer schedules derived work in the same transaction as a message.
type JobEnqueuer interface {
	EnqueueMessageMemoryExtraction(ctx context.Context, tx *gorm.DB, userID, messageID, contentHash string, runAt time.Time) error
}

// NewService creates the Companion service.
func NewService(
	repo *Repository,
	retrieval *journal.RetrievalService,
	memories MemoryRetriever,
	jobs JobEnqueuer,
	llm ai.LLMProvider,
	limiter *ratelimit.Limiter,
	limits Limits,
	model string,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:      repo,
		retrieval: retrieval,
		memories:  memories,
		jobs:      jobs,
		llm:       llm,
		limiter:   limiter,
		limits:    limits,
		model:     model,
		log:       log,
		now:       time.Now,
	}
}

// CreateConversation creates a Companion session.
func (s *Service) CreateConversation(ctx context.Context, userID string, title *string) (*ConversationView, error) {
	if title != nil && utf8.RuneCountInString(*title) > 200 {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "title",
			Code:    "TOO_LONG",
			Message: "Title must be at most 200 characters.",
		})
	}

	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()

	conversation := &Conversation{
		ID:        id,
		UserID:    userID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateConversation(ctx, conversation); err != nil {
		return nil, err
	}

	view := newConversationView(conversation)
	return &view, nil
}

// ListConversations returns a page of conversations.
func (s *Service) ListConversations(ctx context.Context, userID, cursor string, limit int) (*ConversationPage, error) {
	limit = clampLimit(limit)

	var (
		after   *time.Time
		afterID string
	)
	if cursor != "" {
		updatedAt, id, err := decodeConversationCursor(cursor)
		if err != nil {
			return nil, invalidCursor("cursor")
		}
		after = &updatedAt
		afterID = id
	}

	conversations, err := s.repo.ListConversations(ctx, userID, after, afterID, limit)
	if err != nil {
		return nil, err
	}

	hasMore := len(conversations) > limit
	if hasMore {
		conversations = conversations[:limit]
	}

	items := make([]ConversationView, 0, len(conversations))
	for index := range conversations {
		items = append(items, newConversationView(&conversations[index]))
	}

	var nextCursor *string
	if hasMore && len(conversations) > 0 {
		last := conversations[len(conversations)-1]
		value := encodeConversationCursor(last.UpdatedAt, last.ID)
		nextCursor = &value
	}

	return &ConversationPage{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// GetConversation returns one owned conversation.
func (s *Service) GetConversation(ctx context.Context, userID, conversationID string) (*ConversationView, error) {
	conversation, err := s.repo.GetConversation(ctx, userID, conversationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, conversationNotFound()
	}
	if err != nil {
		return nil, err
	}
	view := newConversationView(conversation)
	return &view, nil
}

// DeleteConversation soft-deletes a conversation.
func (s *Service) DeleteConversation(ctx context.Context, userID, conversationID string) error {
	now := s.now().UTC()
	var deleted bool

	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		removed, err := s.repo.SoftDeleteConversationTx(tx, userID, conversationID, now)
		if err != nil {
			return err
		}
		if !removed {
			return nil
		}
		deleted = true
		return memory.ReconcileConversationSourcesTx(ctx, tx, userID, conversationID, now)
	})
	if err != nil {
		return err
	}
	if !deleted {
		return conversationNotFound()
	}
	return nil
}

// ListMessages returns a page of messages in ascending chronological order.
func (s *Service) ListMessages(ctx context.Context, userID, conversationID, before string, limit int) (*MessagePage, error) {
	if _, err := s.repo.GetConversation(ctx, userID, conversationID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, conversationNotFound()
		}
		return nil, err
	}

	limit = clampLimit(limit)

	var (
		beforeTime *time.Time
		beforeID   string
	)
	if before != "" {
		createdAt, id, err := decodeMessageCursor(before)
		if err != nil {
			return nil, invalidCursor("before")
		}
		beforeTime = &createdAt
		beforeID = id
	}

	messages, err := s.repo.ListMessages(ctx, conversationID, beforeTime, beforeID, limit)
	if err != nil {
		return nil, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	var nextCursor *string
	if hasMore && len(messages) > 0 {
		last := messages[len(messages)-1]
		value := encodeMessageCursor(last.CreatedAt, last.ID)
		nextCursor = &value
	}

	items := make([]MessageView, 0, len(messages))
	for index := len(messages) - 1; index >= 0; index-- {
		items = append(items, newMessageView(&messages[index]))
	}

	return &MessagePage{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// SendMessage persists the user message, generates a Companion response, and
// stores the assistant message.
func (s *Service) SendMessage(ctx context.Context, userID, conversationID, idempotencyKey, content string) (*SendPayload, error) {
	if err := validateContent(content); err != nil {
		return nil, err
	}
	if err := s.enforceRateLimit(ctx, userID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetConversation(ctx, userID, conversationID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, conversationNotFound()
		}
		return nil, err
	}

	now := s.now().UTC()
	endpoint := "POST /conversations/" + conversationID + "/messages"
	requestHash := hashContent(content)

	recordID, err := ids.New()
	if err != nil {
		return nil, err
	}
	messageID, err := ids.New()
	if err != nil {
		return nil, err
	}

	userMessage := &Message{
		ID:             messageID,
		ConversationID: conversationID,
		UserID:         userID,
		Role:           RoleUser,
		Content:        content,
		IdempotencyKey: &idempotencyKey,
		CreatedAt:      now,
	}

	record := &IdempotencyRecord{
		ID:          recordID,
		UserID:      userID,
		Key:         idempotencyKey,
		Endpoint:    endpoint,
		RequestHash: requestHash,
		ExpiresAt:   now.Add(idempotencyTTL),
		CreatedAt:   now,
	}

	var (
		replay        *SendPayload
		persistedUser *Message
	)
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		claimed, err := s.repo.ClaimIdempotency(tx, record)
		if err != nil {
			return err
		}

		if !claimed {
			existing, err := s.repo.IdempotencyRecord(tx, userID, endpoint, idempotencyKey)
			if err != nil {
				return err
			}
			if existing.RequestHash != requestHash {
				return apierrors.IdempotencyKeyReused()
			}
			if !existing.ExpiresAt.After(now) {
				return apierrors.Conflict("The idempotency key has expired. Use a new key.")
			}
			if existing.ResponseStatus != nil && len(existing.ResponseBody) > 0 {
				var stored SendPayload
				if err := json.Unmarshal(existing.ResponseBody, &stored); err != nil {
					return err
				}
				replay = &stored
				return nil
			}
			if existing.CreatedAt.After(now.Add(-pendingIdempotencyAge)) {
				return apierrors.Conflict("The message is still being processed.")
			}
			if err := s.repo.DeleteIdempotencyTx(tx, existing.ID); err != nil {
				return err
			}
			claimed, err = s.repo.ClaimIdempotency(tx, record)
			if err != nil {
				return err
			}
			if !claimed {
				return apierrors.Conflict("The message is still being processed.")
			}
		}

		inserted, err := s.repo.CreateMessageTx(tx, userMessage)
		if err != nil {
			return err
		}
		if inserted {
			persistedUser = userMessage
			return nil
		}

		existingMessage, err := s.repo.FindMessageByIdempotencyKey(tx, conversationID, idempotencyKey)
		if err != nil {
			return err
		}
		if existingMessage.Content != content {
			return apierrors.IdempotencyKeyReused()
		}
		persistedUser = existingMessage
		return nil
	})
	if err != nil {
		return nil, err
	}
	if replay != nil {
		return replay, nil
	}
	if persistedUser == nil {
		return nil, apierrors.Internal()
	}

	recent, err := s.repo.RecentMessages(ctx, conversationID, 20)
	if err != nil {
		return nil, err
	}
	reverseMessages(recent)

	language, style, err := s.repo.ProfilePreferences(ctx, userID)
	if err != nil {
		return nil, err
	}

	retrieved, err := s.retrieval.Retrieve(ctx, userID, content, 8)
	if err != nil {
		s.log.WarnContext(ctx, "companion context retrieval failed",
			slog.String("conversation_id", conversationID),
			slog.String("error", err.Error()),
		)
		retrieved = nil
	}

	memoriesUsed, err := s.retrieveMemories(ctx, userID, content)
	if err != nil {
		s.log.WarnContext(ctx, "companion memory retrieval failed",
			slog.String("conversation_id", conversationID),
			slog.String("error", err.Error()),
		)
		memoriesUsed = nil
	}

	systemPrompt, err := BuildSystemPrompt(language, style, retrieved, memoriesUsed)
	if err != nil {
		return nil, err
	}

	history := BuildMessages(recent, historyTokenBudget)
	if len(history) == 0 {
		history = []ai.Message{{Role: ai.RoleUser, Content: content}}
	}

	response, err := s.llm.Generate(ctx, ai.GenerateRequest{
		Model:       s.model,
		System:      systemPrompt,
		Messages:    history,
		Temperature: 0.7,
		MaxTokens:   maxOutputTokens,
		SessionID:   conversationID,
	})
	if err != nil {
		_ = s.repo.DeleteIdempotency(ctx, record.ID)
		s.log.WarnContext(ctx, "companion generation failed",
			slog.String("conversation_id", conversationID),
			slog.String("error", err.Error()),
		)
		return nil, upstreamError(err)
	}

	assistantID, err := ids.New()
	if err != nil {
		return nil, err
	}
	assistantTime := s.now().UTC()

	metadata, err := json.Marshal(map[string]any{
		"model":          response.Model,
		"prompt_version": prompts.CompanionVersion,
		"input_tokens":   response.Usage.InputTokens,
		"output_tokens":  response.Usage.OutputTokens,
		"journals_used":  len(retrieved),
		"memories_used":  len(memoriesUsed),
	})
	if err != nil {
		return nil, err
	}

	assistant := &Message{
		ID:             assistantID,
		ConversationID: conversationID,
		UserID:         userID,
		Role:           RoleAssistant,
		Content:        response.Content,
		Metadata:       metadata,
		CreatedAt:      assistantTime,
	}

	payload := &SendPayload{
		UserMessage:      newMessageView(persistedUser),
		AssistantMessage: newMessageView(assistant),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if _, err := s.repo.CreateMessageTx(tx, assistant); err != nil {
			return err
		}
		if err := s.repo.UpdateConversationTimestamp(tx, conversationID, assistantTime); err != nil {
			return err
		}
		if err := s.repo.CompleteIdempotency(tx, record.ID, 200, body); err != nil {
			return err
		}
		if s.jobs == nil {
			return nil
		}
		return s.jobs.EnqueueMessageMemoryExtraction(
			ctx,
			tx,
			userID,
			persistedUser.ID,
			hashContent(persistedUser.Content),
			assistantTime,
		)
	})
	if err != nil {
		_ = s.repo.DeleteIdempotency(ctx, record.ID)
		return nil, err
	}

	s.log.InfoContext(ctx, "companion message completed",
		slog.String("conversation_id", conversationID),
		slog.String("user_id", userID),
		slog.String("model", response.Model),
		slog.Int("input_tokens", response.Usage.InputTokens),
		slog.Int("output_tokens", response.Usage.OutputTokens),
	)

	return payload, nil
}

func (s *Service) retrieveMemories(ctx context.Context, userID, content string) ([]memory.RetrievedMemory, error) {
	if s.memories == nil {
		return nil, nil
	}
	return s.memories.Retrieve(ctx, userID, content, 5)
}

func (s *Service) enforceRateLimit(ctx context.Context, userID string) error {
	if s.limiter == nil {
		return nil
	}
	decision, err := s.limiter.Check(ctx, companionRateLimitKey, userID, s.limits.CompanionPerMinute, time.Minute)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apierrors.RateLimited(decision.RetryAfter)
	}
	return s.limiter.Record(ctx, companionRateLimitKey, userID)
}

func validateContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "content",
			Code:    "REQUIRED",
			Message: "Message content must not be empty.",
		})
	}
	if utf8.RuneCountInString(content) > 20000 {
		return apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "content",
			Code:    "TOO_LONG",
			Message: "Message content must be at most 20000 characters.",
		})
	}
	return nil
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > conversationPageLimit {
		return conversationPageLimit
	}
	return limit
}

func reverseMessages(messages []Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func conversationNotFound() *apierrors.Error {
	return apierrors.NotFound("The conversation was not found.")
}

func invalidCursor(field string) *apierrors.Error {
	return apierrors.Validation("The request is invalid.", apierrors.Detail{
		Field:   field,
		Code:    "INVALID",
		Message: "The cursor is invalid.",
	})
}

func upstreamError(err error) *apierrors.Error {
	if errors.Is(err, ai.ErrTimeout) {
		return apierrors.UpstreamTimeout()
	}
	return apierrors.BadGateway("The companion is temporarily unavailable.")
}
