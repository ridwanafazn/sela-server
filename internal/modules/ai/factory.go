package ai

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"

	"github.com/ridwanafazn/sela-server/internal/config"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// NewLLM builds the configured language provider.
func NewLLM(cfg config.AI) (LLMProvider, error) {
	switch cfg.Provider {
	case "fake":
		return NewFakeLLM(), nil
	case "openai":
		return NewOpenAILLM(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Timeout, cfg.MaxRetries), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider %q", cfg.Provider)
	}
}

// NewEmbedding builds the configured embedding provider.
func NewEmbedding(ctx context.Context, cfg config.AI) (EmbeddingProvider, error) {
	switch cfg.EmbeddingProvider {
	case "fake":
		return NewFakeEmbedding(cfg.EmbeddingDimension), nil
	case "openai":
		return NewOpenAIEmbedding(
			cfg.EmbeddingBaseURL,
			cfg.EmbeddingAPIKey,
			cfg.EmbeddingModel,
			cfg.EmbeddingDimension,
			cfg.EmbeddingTimeout,
			cfg.MaxRetries,
		), nil
	case "vertex":
		tokens, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("create Vertex AI token source: %w", err)
		}
		return NewVertexEmbedding(
			cfg.VertexProjectID,
			cfg.VertexRegion,
			cfg.VertexEmbeddingModel,
			cfg.VertexBaseURL,
			cfg.EmbeddingDimension,
			tokens,
			cfg.EmbeddingTimeout,
			cfg.MaxRetries,
		), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider %q", cfg.EmbeddingProvider)
	}
}
