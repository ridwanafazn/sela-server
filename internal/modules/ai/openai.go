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
)

// OpenAI implements the LLM and embedding interfaces for OpenAI-compatible
// APIs such as OpenRouter and compatible gateways.
type OpenAI struct {
	baseURL            string
	apiKey             string
	defaultModel       string
	embeddingDimension int
	httpClient         *http.Client
	timeout            time.Duration
	maxRetries         int
}

// NewOpenAILLM creates an OpenAI-compatible text provider.
func NewOpenAILLM(baseURL, apiKey, model string, timeout time.Duration, maxRetries int) *OpenAI {
	return &OpenAI{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		defaultModel: model,
		httpClient:   &http.Client{},
		timeout:      timeout,
		maxRetries:   maxRetries,
	}
}

// NewOpenAIEmbedding creates an OpenAI-compatible embedding provider.
func NewOpenAIEmbedding(baseURL, apiKey, model string, dimension int, timeout time.Duration, maxRetries int) *OpenAI {
	provider := NewOpenAILLM(baseURL, apiKey, model, timeout, maxRetries)
	provider.embeddingDimension = dimension
	return provider
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Generate calls the chat completions endpoint.
func (o *OpenAI) Generate(ctx context.Context, request GenerateRequest) (*GenerateResponse, error) {
	model := request.Model
	if model == "" {
		model = o.defaultModel
	}

	messages := make([]openAIMessage, 0, len(request.Messages)+1)
	if request.System != "" {
		messages = append(messages, openAIMessage{Role: RoleSystem, Content: request.System})
	}
	for _, message := range request.Messages {
		messages = append(messages, openAIMessage{Role: message.Role, Content: message.Content})
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if request.MaxTokens > 0 {
		payload["max_tokens"] = request.MaxTokens
	}
	if request.Temperature > 0 {
		payload["temperature"] = request.Temperature
	}
	if len(request.ResponseSchema) > 0 {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}

	body, err := o.post(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, err
	}

	var response chatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: decode chat response: %v", ErrInvalidResponse, err)
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("%w: empty completion", ErrInvalidResponse)
	}

	responseModel := response.Model
	if responseModel == "" {
		responseModel = model
	}

	return &GenerateResponse{
		Content:      response.Choices[0].Message.Content,
		Model:        responseModel,
		FinishReason: response.Choices[0].FinishReason,
		Usage: Usage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
		},
	}, nil
}

type embeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
}

// Embed calls the embeddings endpoint.
func (o *OpenAI) Embed(ctx context.Context, request EmbeddingRequest) (*EmbeddingResponse, error) {
	model := request.Model
	if model == "" {
		model = o.defaultModel
	}

	body, err := o.post(ctx, "/embeddings", map[string]any{
		"model":           model,
		"input":           request.Texts,
		"encoding_format": "float",
	})
	if err != nil {
		return nil, err
	}

	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: decode embedding response: %v", ErrInvalidResponse, err)
	}
	if len(response.Data) != len(request.Texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, got %d", ErrInvalidResponse, len(request.Texts), len(response.Data))
	}

	vectors := make([][]float32, 0, len(response.Data))
	for _, item := range response.Data {
		if o.embeddingDimension > 0 && len(item.Embedding) != o.embeddingDimension {
			return nil, fmt.Errorf("%w: got %d dimensions, want %d", ErrDimensionMismatch, len(item.Embedding), o.embeddingDimension)
		}
		vectors = append(vectors, item.Embedding)
	}

	responseModel := response.Model
	if responseModel == "" {
		responseModel = model
	}

	return &EmbeddingResponse{
		Model:      responseModel,
		Embeddings: vectors,
		Usage:      Usage{InputTokens: response.Usage.PromptTokens},
	}, nil
}

func (o *OpenAI) post(ctx context.Context, path string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request: %v", ErrInvalidRequest, err)
	}

	attempts := o.maxRetries + 1
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

		body, err := o.attempt(ctx, path, encoded)
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

func (o *OpenAI) attempt(ctx context.Context, path string, payload []byte) ([]byte, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, o.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrInvalidRequest, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if o.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	response, err := o.httpClient.Do(request)
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
