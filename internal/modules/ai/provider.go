// Package ai defines provider-independent language and embedding interfaces.
package ai

import (
	"context"
	"encoding/json"
	"errors"
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one conversation turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateRequest is a provider-independent generation request.
type GenerateRequest struct {
	Model          string
	System         string
	Messages       []Message
	Temperature    float64
	MaxTokens      int
	ResponseSchema json.RawMessage
	SessionID      string
}

// Usage records token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// GenerateResponse is a provider-independent generation result.
type GenerateResponse struct {
	Content      string
	Model        string
	FinishReason string
	Usage        Usage
}

// LLMProvider generates text.
type LLMProvider interface {
	Generate(ctx context.Context, request GenerateRequest) (*GenerateResponse, error)
}

// EmbeddingRequest is a provider-independent embedding request.
type EmbeddingRequest struct {
	Model string
	Texts []string
}

// EmbeddingResponse is a provider-independent embedding result.
type EmbeddingResponse struct {
	Model      string
	Embeddings [][]float32
	Usage      Usage
}

// EmbeddingProvider generates embeddings.
type EmbeddingProvider interface {
	Embed(ctx context.Context, request EmbeddingRequest) (*EmbeddingResponse, error)
}

// Provider error categories. Callers map these to safe API errors.
var (
	ErrInvalidRequest    = errors.New("ai: invalid request")
	ErrAuthentication    = errors.New("ai: authentication failed")
	ErrRateLimited       = errors.New("ai: rate limited")
	ErrUnavailable       = errors.New("ai: provider unavailable")
	ErrTimeout           = errors.New("ai: provider timeout")
	ErrInvalidResponse   = errors.New("ai: invalid provider response")
	ErrDimensionMismatch = errors.New("ai: embedding dimension mismatch")
)

// Retryable reports whether the error category is transient.
func Retryable(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrTimeout)
}

// ErrorCategory returns a stable, non-sensitive label for usage records.
func ErrorCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAuthentication):
		return "authentication"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	case errors.Is(err, ErrInvalidResponse):
		return "invalid_response"
	case errors.Is(err, ErrDimensionMismatch):
		return "dimension_mismatch"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	default:
		return "unknown"
	}
}
