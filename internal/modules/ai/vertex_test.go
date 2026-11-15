package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestVertexEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/test-project/locations/test-region/publishers/google/models/text-embedding-004:predict" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}

		var request struct {
			Instances []struct {
				Content string `json:"content"`
			} `json:"instances"`
			Parameters struct {
				OutputDimensionality int  `json:"outputDimensionality"`
				AutoTruncate         bool `json:"autoTruncate"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Instances) != 2 {
			t.Errorf("instances = %d, want 2", len(request.Instances))
		}
		if request.Parameters.OutputDimensionality != 3 {
			t.Errorf("outputDimensionality = %d, want 3", request.Parameters.OutputDimensionality)
		}
		if !request.Parameters.AutoTruncate {
			t.Error("autoTruncate = false, want true")
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"predictions": []map[string]any{
				{"embeddings": map[string]any{"values": []float32{0.1, 0.2, 0.3}}},
				{"embeddings": map[string]any{"values": []float32{0.4, 0.5, 0.6}}},
			},
		})
	}))
	defer server.Close()

	provider := NewVertexEmbedding(
		"test-project",
		"test-region",
		"text-embedding-004",
		server.URL,
		3,
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
		time.Second,
		0,
	)

	response, err := provider.Embed(context.Background(), EmbeddingRequest{Texts: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(response.Embeddings) != 2 || len(response.Embeddings[0]) != 3 {
		t.Errorf("embeddings shape = %d x %d", len(response.Embeddings), len(response.Embeddings[0]))
	}
}

func TestVertexEmbeddingDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"predictions": []map[string]any{
				{"embeddings": map[string]any{"values": []float32{0.1, 0.2}}},
			},
		})
	}))
	defer server.Close()

	provider := NewVertexEmbedding(
		"test-project",
		"test-region",
		"text-embedding-004",
		server.URL,
		768,
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
		time.Second,
		0,
	)

	_, err := provider.Embed(context.Background(), EmbeddingRequest{Texts: []string{"one"}})
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Errorf("error = %v, want ErrDimensionMismatch", err)
	}
}
