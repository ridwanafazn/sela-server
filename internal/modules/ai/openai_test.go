package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") != "sela-server/1.0" {
			t.Errorf("user-agent = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("x-opencode-session") != "session-123" {
			t.Errorf("x-opencode-session = %q", r.Header.Get("x-opencode-session"))
		}

		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "test-model" || len(request.Messages) != 2 {
			t.Errorf("request = %+v", request)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{{
				"message":       map[string]string{"content": "A calm response."},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 4},
		})
	}))
	defer server.Close()

	provider := NewOpenAILLM(server.URL, "test-key", "test-model", 2*time.Second, 0)
	response, err := provider.Generate(context.Background(), GenerateRequest{
		System:    "System",
		Messages:  []Message{{Role: RoleUser, Content: "Hello"}},
		SessionID: "session-123",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content != "A calm response." || response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 4 {
		t.Errorf("response = %+v", response)
	}
}

func TestOpenAIRetriesRateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]string{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAILLM(server.URL, "test-key", "test-model", 2*time.Second, 1)
	if _, err := provider.Generate(context.Background(), GenerateRequest{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestOpenAIAuthenticationErrorIsNotRetried(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := NewOpenAILLM(server.URL, "bad-key", "test-model", time.Second, 2)
	_, err := provider.Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("Generate returned nil error")
	}
	if !errors.Is(err, ErrAuthentication) {
		t.Errorf("error = %v, want ErrAuthentication", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry)", calls)
	}
}

func TestOpenAIEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %s, want /embeddings", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "embedding-model",
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2, 0.3}},
				{"embedding": []float32{0.4, 0.5, 0.6}},
			},
			"usage": map[string]int{"prompt_tokens": 7},
		})
	}))
	defer server.Close()

	provider := NewOpenAIEmbedding(server.URL, "test-key", "embedding-model", 3, time.Second, 0)
	response, err := provider.Embed(context.Background(), EmbeddingRequest{Texts: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(response.Embeddings) != 2 || len(response.Embeddings[0]) != 3 {
		t.Errorf("embeddings shape = %d x %d", len(response.Embeddings), len(response.Embeddings[0]))
	}
	if response.Usage.InputTokens != 7 {
		t.Errorf("usage = %d, want 7", response.Usage.InputTokens)
	}
}

func TestOpenAIEmbeddingDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	provider := NewOpenAIEmbedding(server.URL, "test-key", "embedding-model", 768, time.Second, 0)
	_, err := provider.Embed(context.Background(), EmbeddingRequest{Texts: []string{"one"}})
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Errorf("error = %v, want ErrDimensionMismatch", err)
	}
}

func TestOpenAIInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	provider := NewOpenAILLM(server.URL, "test-key", "test-model", time.Second, 0)
	_, err := provider.Generate(context.Background(), GenerateRequest{})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("error = %v, want ErrInvalidResponse", err)
	}
}
