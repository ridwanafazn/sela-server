package ai

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"time"
)

// FakeLLM is a deterministic local provider. It never calls an external
// service.
type FakeLLM struct {
	Reply           string
	StructuredReply string
	Err             error
	Delay           time.Duration
	Model           string
}

// NewFakeLLM creates a fake provider with a safe default response.
func NewFakeLLM() *FakeLLM {
	return &FakeLLM{
		Reply:           "I hear you. What feels most important right now?",
		StructuredReply: `{"memories":[]}`,
		Model:           "fake-chat",
	}
}

// Generate returns the configured reply.
func (f *FakeLLM) Generate(ctx context.Context, request GenerateRequest) (*GenerateResponse, error) {
	if f.Delay > 0 {
		select {
		case <-time.After(f.Delay):
		case <-ctx.Done():
			return nil, errors.Join(ErrTimeout, ctx.Err())
		}
	}
	if f.Err != nil {
		return nil, f.Err
	}

	content := f.Reply
	if len(request.ResponseSchema) > 0 {
		content = f.StructuredReply
		if content == "" {
			content = `{"memories":[]}`
		}
	}
	if content == "" {
		content = "I hear you."
	}

	input := approximateTokens(request.System)
	for _, message := range request.Messages {
		input += approximateTokens(message.Content)
	}

	model := request.Model
	if model == "" {
		model = f.Model
	}

	return &GenerateResponse{
		Content:      content,
		Model:        model,
		FinishReason: "stop",
		Usage: Usage{
			InputTokens:  input,
			OutputTokens: approximateTokens(content),
		},
	}, nil
}

// FakeEmbedding is a deterministic 768-dimension embedding provider.
type FakeEmbedding struct {
	Dimension int
	Err       error
}

// NewFakeEmbedding creates a fake embedding provider.
func NewFakeEmbedding(dimension int) *FakeEmbedding {
	if dimension <= 0 {
		dimension = 768
	}
	return &FakeEmbedding{Dimension: dimension}
}

// Embed returns normalized vectors derived from SHA-256 of each text.
func (f *FakeEmbedding) Embed(ctx context.Context, request EmbeddingRequest) (*EmbeddingResponse, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrTimeout, err)
	}

	dimension := f.Dimension
	if dimension <= 0 {
		dimension = 768
	}

	vectors := make([][]float32, 0, len(request.Texts))
	for _, text := range request.Texts {
		hash := sha256.Sum256([]byte(text))
		vector := make([]float32, dimension)

		var sumSquares float64
		for index := range vector {
			value := float64(hash[index%len(hash)])/127.5 - 1
			vector[index] = float32(value)
			sumSquares += value * value
		}

		norm := math.Sqrt(sumSquares)
		if norm > 0 {
			for index := range vector {
				vector[index] = float32(float64(vector[index]) / norm)
			}
		}

		vectors = append(vectors, vector)
	}

	inputTokens := 0
	for _, text := range request.Texts {
		inputTokens += approximateTokens(text)
	}

	model := request.Model
	if model == "" {
		model = "fake-embedding-768"
	}

	return &EmbeddingResponse{
		Model:      model,
		Embeddings: vectors,
		Usage:      Usage{InputTokens: inputTokens},
	}, nil
}

func approximateTokens(text string) int {
	tokens := len(text) / 4
	if tokens < 1 && text != "" {
		return 1
	}
	return tokens
}
