package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// VertexEmbedding calls the Vertex AI prediction endpoint for embeddings.
type VertexEmbedding struct {
	projectID  string
	region     string
	model      string
	baseURL    string
	dimension  int
	tokens     oauth2.TokenSource
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int
}

// NewVertexEmbedding creates a Vertex AI embedding provider. baseURL can be
// overridden for tests; when empty, the regional Vertex endpoint is used.
func NewVertexEmbedding(
	projectID, region, model, baseURL string,
	dimension int,
	tokens oauth2.TokenSource,
	timeout time.Duration,
	maxRetries int,
) *VertexEmbedding {
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", region)
	}

	return &VertexEmbedding{
		projectID:  projectID,
		region:     region,
		model:      model,
		baseURL:    strings.TrimRight(baseURL, "/"),
		dimension:  dimension,
		tokens:     tokens,
		httpClient: &http.Client{},
		timeout:    timeout,
		maxRetries: maxRetries,
	}
}

type vertexEmbeddingResponse struct {
	Predictions []struct {
		Embeddings struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	} `json:"predictions"`
}

// Embed requests embeddings for each text. Vertex limits instances per
// request, so texts are sent in bounded batches.
func (v *VertexEmbedding) Embed(ctx context.Context, request EmbeddingRequest) (*EmbeddingResponse, error) {
	if len(request.Texts) == 0 {
		return &EmbeddingResponse{Model: request.Model, Embeddings: [][]float32{}}, nil
	}

	model := request.Model
	if model == "" {
		model = v.model
	}

	endpoint := fmt.Sprintf(
		"%s/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		v.baseURL, v.projectID, v.region, model,
	)

	vectors := make([][]float32, 0, len(request.Texts))
	inputTokens := 0

	const batchSize = 250
	for start := 0; start < len(request.Texts); start += batchSize {
		end := start + batchSize
		if end > len(request.Texts) {
			end = len(request.Texts)
		}

		instances := make([]map[string]string, 0, end-start)
		for _, text := range request.Texts[start:end] {
			instances = append(instances, map[string]string{"content": text})
			inputTokens += approximateTokens(text)
		}

		payload := map[string]any{"instances": instances}
		if v.dimension > 0 {
			payload["parameters"] = map[string]any{
				"outputDimensionality": v.dimension,
				"autoTruncate":         true,
			}
		}
		body, err := v.post(ctx, endpoint, payload)
		if err != nil {
			return nil, err
		}

		var response vertexEmbeddingResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("%w: decode Vertex response: %v", ErrInvalidResponse, err)
		}
		if len(response.Predictions) != len(instances) {
			return nil, fmt.Errorf("%w: expected %d predictions, got %d", ErrInvalidResponse, len(instances), len(response.Predictions))
		}

		for _, prediction := range response.Predictions {
			if v.dimension > 0 && len(prediction.Embeddings.Values) != v.dimension {
				return nil, fmt.Errorf("%w: got %d dimensions, want %d", ErrDimensionMismatch, len(prediction.Embeddings.Values), v.dimension)
			}
			vectors = append(vectors, prediction.Embeddings.Values)
		}
	}

	return &EmbeddingResponse{
		Model:      model,
		Embeddings: vectors,
		Usage:      Usage{InputTokens: inputTokens},
	}, nil
}

func (v *VertexEmbedding) post(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request: %v", ErrInvalidRequest, err)
	}

	attempts := v.maxRetries + 1
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			backoff += time.Duration(rand.IntN(250)) * time.Millisecond
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		body, err := v.attempt(ctx, endpoint, encoded)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !Retryable(err) {
			return nil, err
		}
	}

	return nil, lastErr
}

func (v *VertexEmbedding) attempt(ctx context.Context, endpoint string, payload []byte) ([]byte, error) {
	if v.tokens == nil {
		return nil, fmt.Errorf("%w: no token source", ErrAuthentication)
	}

	token, err := v.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthentication, err)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrInvalidRequest, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)

	response, err := v.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrUnavailable, err)
	}

	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return body, nil
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: status %d", ErrAuthentication, response.StatusCode)
	case response.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: status %d", ErrRateLimited, response.StatusCode)
	case response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusGatewayTimeout:
		return nil, fmt.Errorf("%w: status %d", ErrTimeout, response.StatusCode)
	case response.StatusCode >= 500:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, response.StatusCode)
	default:
		return nil, fmt.Errorf("%w: status %d", ErrInvalidRequest, response.StatusCode)
	}
}
