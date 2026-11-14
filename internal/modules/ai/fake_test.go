package ai

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestFakeEmbeddingIsDeterministicAndNormalized(t *testing.T) {
	provider := NewFakeEmbedding(768)
	ctx := context.Background()

	first, err := provider.Embed(ctx, EmbeddingRequest{Texts: []string{"I could not sleep"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	second, err := provider.Embed(ctx, EmbeddingRequest{Texts: []string{"I could not sleep"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(first.Embeddings) != 1 || len(first.Embeddings[0]) != 768 {
		t.Fatalf("embedding shape = %d x %d, want 1 x 768", len(first.Embeddings), len(first.Embeddings[0]))
	}
	if first.Embeddings[0][0] != second.Embeddings[0][0] {
		t.Error("the same text produced different embeddings")
	}

	var sumSquares float64
	for _, value := range first.Embeddings[0] {
		sumSquares += float64(value) * float64(value)
	}
	if math.Abs(math.Sqrt(sumSquares)-1) > 1e-5 {
		t.Errorf("vector is not normalized: norm=%f", math.Sqrt(sumSquares))
	}

	other, err := provider.Embed(ctx, EmbeddingRequest{Texts: []string{"A different entry"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if other.Embeddings[0][0] == first.Embeddings[0][0] {
		t.Error("different texts produced the same embedding start")
	}
}

func TestFakeLLMReturnsDeterministicReply(t *testing.T) {
	provider := NewFakeLLM()

	response, err := provider.Generate(context.Background(), GenerateRequest{
		System:   "You are SELA.",
		Messages: []Message{{Role: RoleUser, Content: "I feel stuck."}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content == "" || response.FinishReason != "stop" {
		t.Errorf("response = %+v", response)
	}
	if response.Usage.InputTokens == 0 {
		t.Error("input usage is zero")
	}
}

func TestFakeLLMRespectsCancellation(t *testing.T) {
	provider := &FakeLLM{Delay: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := provider.Generate(ctx, GenerateRequest{})
	if err == nil {
		t.Fatal("Generate returned nil error after context cancellation")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}
