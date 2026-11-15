//go:build integration

package integration_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type conversationPayload struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type conversationEnvelope struct {
	Data  conversationPayload `json:"data"`
	Error errorPayload        `json:"error"`
}

type conversationListEnvelope struct {
	Data       []conversationPayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Error errorPayload `json:"error"`
}

type messagePayload struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type messageEnvelope struct {
	Data  messagePayload `json:"data"`
	Error errorPayload   `json:"error"`
}

type messageListEnvelope struct {
	Data       []messagePayload `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	Error errorPayload `json:"error"`
}

type sendPayload struct {
	UserMessage      messagePayload `json:"user_message"`
	AssistantMessage messagePayload `json:"assistant_message"`
}

type sendEnvelope struct {
	Data  sendPayload  `json:"data"`
	Error errorPayload `json:"error"`
}

func createConversationAPI(t *testing.T, router *gin.Engine, token string, body any) apiResult {
	t.Helper()

	return do(t, router, http.MethodPost, "/api/v1/conversations", requestOptions{
		body:  body,
		token: token,
		ip:    "203.0.113.230",
	})
}

func sendMessageAPI(t *testing.T, router *gin.Engine, token, conversationID, key, content string) apiResult {
	t.Helper()

	headers := map[string]string{}
	if key != "" {
		headers["Idempotency-Key"] = key
	}

	return do(t, router, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", requestOptions{
		token:   token,
		ip:      "203.0.113.230",
		headers: headers,
		body:    map[string]any{"content": content},
	})
}

func mustCreateConversation(t *testing.T, router *gin.Engine, token, title string) conversationPayload {
	t.Helper()

	result := createConversationAPI(t, router, token, map[string]any{"title": title})
	if result.Status != http.StatusCreated {
		t.Fatalf("create conversation = %d: %s", result.Status, result.Body)
	}
	return decodeJSON[conversationEnvelope](t, result.Body).Data
}

func TestCompanionFlow(t *testing.T) {
	router, db := newAPI(t, testConfig())
	user := newTestUser(t, router)

	created := mustCreateConversation(t, router, user.AccessToken, "Reflection")
	if created.Title == nil || *created.Title != "Reflection" {
		t.Errorf("title = %v, want Reflection", created.Title)
	}

	get := do(t, router, http.MethodGet, "/api/v1/conversations/"+created.ID, requestOptions{token: user.AccessToken})
	if get.Status != http.StatusOK {
		t.Fatalf("get conversation = %d: %s", get.Status, get.Body)
	}

	key := uuid.NewString()
	content := "I feel stuck with work and cannot focus."
	first := sendMessageAPI(t, router, user.AccessToken, created.ID, key, content)
	if first.Status != http.StatusOK {
		t.Fatalf("send message = %d: %s", first.Status, first.Body)
	}

	sent := decodeJSON[sendEnvelope](t, first.Body).Data
	if sent.UserMessage.Role != "user" || sent.UserMessage.Content != content {
		t.Errorf("user message = %+v", sent.UserMessage)
	}
	if sent.AssistantMessage.Role != "assistant" || sent.AssistantMessage.Content == "" {
		t.Errorf("assistant message = %+v", sent.AssistantMessage)
	}

	replay := sendMessageAPI(t, router, user.AccessToken, created.ID, key, content)
	if replay.Status != http.StatusOK {
		t.Fatalf("replay = %d: %s", replay.Status, replay.Body)
	}
	replayed := decodeJSON[sendEnvelope](t, replay.Body).Data
	if replayed.UserMessage.ID != sent.UserMessage.ID || replayed.AssistantMessage.ID != sent.AssistantMessage.ID {
		t.Error("idempotent replay returned different messages")
	}

	conflict := sendMessageAPI(t, router, user.AccessToken, created.ID, key, "A different message.")
	if conflict.Status != http.StatusConflict {
		t.Fatalf("reused key = %d, want 409: %s", conflict.Status, conflict.Body)
	}
	if envelope := decodeJSON[errorEnvelope](t, conflict.Body); envelope.Error.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Errorf("error code = %q, want IDEMPOTENCY_KEY_REUSED", envelope.Error.Code)
	}

	messages := decodeJSON[messageListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/conversations/"+created.ID+"/messages", requestOptions{token: user.AccessToken}).Body)
	if len(messages.Data) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages.Data))
	}
	if messages.Data[0].Role != "user" || messages.Data[1].Role != "assistant" {
		t.Errorf("message order = %s, %s", messages.Data[0].Role, messages.Data[1].Role)
	}

	conversations := decodeJSON[conversationListEnvelope](t, do(t, router, http.MethodGet,
		"/api/v1/conversations", requestOptions{token: user.AccessToken}).Body)
	if len(conversations.Data) != 1 {
		t.Errorf("conversations = %d, want 1", len(conversations.Data))
	}

	var metadata []byte
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT metadata FROM messages WHERE id = $1`, sent.AssistantMessage.ID,
	).Scan(&metadata); err != nil {
		t.Fatalf("load assistant metadata: %v", err)
	}
	if len(metadata) == 0 {
		t.Error("assistant metadata is empty")
	}

	var usageCount, inputTokens, outputTokens int
	if err := db.SQL.QueryRowContext(t.Context(),
		`SELECT count(*), COALESCE(max(input_tokens), 0), COALESCE(max(output_tokens), 0)
		 FROM ai_usage_events
		 WHERE user_id = $1 AND operation = 'companion' AND success = true`,
		user.User.ID,
	).Scan(&usageCount, &inputTokens, &outputTokens); err != nil {
		t.Fatalf("load companion usage: %v", err)
	}
	if usageCount != 1 || inputTokens == 0 || outputTokens == 0 {
		t.Errorf("companion usage = (count %d, input %d, output %d)", usageCount, inputTokens, outputTokens)
	}

	deleted := do(t, router, http.MethodDelete, "/api/v1/conversations/"+created.ID, requestOptions{token: user.AccessToken})
	if deleted.Status != http.StatusNoContent {
		t.Fatalf("delete conversation = %d: %s", deleted.Status, deleted.Body)
	}

	afterDelete := do(t, router, http.MethodGet, "/api/v1/conversations/"+created.ID, requestOptions{token: user.AccessToken})
	if afterDelete.Status != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", afterDelete.Status)
	}
}

func TestCompanionOwnership(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	owner := newTestUser(t, router)
	stranger := newTestUser(t, router)

	conversation := mustCreateConversation(t, router, owner.AccessToken, "Private")

	if result := do(t, router, http.MethodGet, "/api/v1/conversations/"+conversation.ID, requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger get = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodGet, "/api/v1/conversations/"+conversation.ID+"/messages", requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger messages = %d, want 404", result.Status)
	}
	if result := sendMessageAPI(t, router, stranger.AccessToken, conversation.ID, uuid.NewString(), "hello"); result.Status != http.StatusNotFound {
		t.Errorf("stranger send = %d, want 404", result.Status)
	}
	if result := do(t, router, http.MethodDelete, "/api/v1/conversations/"+conversation.ID, requestOptions{token: stranger.AccessToken}); result.Status != http.StatusNotFound {
		t.Errorf("stranger delete = %d, want 404", result.Status)
	}

	list := decodeJSON[conversationListEnvelope](t, do(t, router, http.MethodGet, "/api/v1/conversations", requestOptions{token: stranger.AccessToken}).Body)
	if len(list.Data) != 0 {
		t.Errorf("stranger conversations = %d, want 0", len(list.Data))
	}
}

func TestCompanionRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimit.CompanionPerMinute = 1
	router, _ := newAPI(t, cfg)
	user := newTestUser(t, router)
	conversation := mustCreateConversation(t, router, user.AccessToken, "Limited")

	first := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "First message.")
	if first.Status != http.StatusOK {
		t.Fatalf("first send = %d: %s", first.Status, first.Body)
	}

	second := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "Second message.")
	if second.Status != http.StatusTooManyRequests {
		t.Fatalf("second send = %d, want 429: %s", second.Status, second.Body)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}
}

func TestCompanionMessagePagination(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)
	conversation := mustCreateConversation(t, router, user.AccessToken, "History")

	for _, content := range []string{"First message.", "Second message.", "Third message."} {
		if result := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), content); result.Status != http.StatusOK {
			t.Fatalf("send = %d: %s", result.Status, result.Body)
		}
	}

	firstPage := do(t, router, http.MethodGet,
		"/api/v1/conversations/"+conversation.ID+"/messages?limit=2",
		requestOptions{token: user.AccessToken},
	)
	if firstPage.Status != http.StatusOK {
		t.Fatalf("first page = %d: %s", firstPage.Status, firstPage.Body)
	}
	pageOne := decodeJSON[messageListEnvelope](t, firstPage.Body)
	if len(pageOne.Data) != 2 || !pageOne.Pagination.HasMore || pageOne.Pagination.NextCursor == nil {
		t.Fatalf("first page = %+v", pageOne)
	}
	if pageOne.Data[0].CreatedAt.After(pageOne.Data[1].CreatedAt) {
		t.Error("messages are not in ascending chronological order")
	}

	secondPage := do(t, router, http.MethodGet,
		"/api/v1/conversations/"+conversation.ID+"/messages?limit=2&before="+url.QueryEscape(*pageOne.Pagination.NextCursor),
		requestOptions{token: user.AccessToken},
	)
	if secondPage.Status != http.StatusOK {
		t.Fatalf("second page = %d: %s", secondPage.Status, secondPage.Body)
	}
	pageTwo := decodeJSON[messageListEnvelope](t, secondPage.Body)
	if len(pageTwo.Data) != 2 {
		t.Errorf("second page size = %d, want 2", len(pageTwo.Data))
	}
}

func TestCompanionAIBudgetExhaustion(t *testing.T) {
	cfg := testConfig()
	cfg.AI.UserMonthlyBudgetUSD = 0.000001
	cfg.AI.InputCostPerMillion = 1000
	cfg.AI.OutputCostPerMillion = 1000
	router, _ := newAPI(t, cfg)
	user := newTestUser(t, router)
	conversation := mustCreateConversation(t, router, user.AccessToken, "Budget")

	first := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "First message.")
	if first.Status != http.StatusOK {
		t.Fatalf("first message = %d: %s", first.Status, first.Body)
	}

	second := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "Second message.")
	if second.Status != http.StatusTooManyRequests {
		t.Fatalf("second message = %d, want 429: %s", second.Status, second.Body)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("budget 429 response is missing Retry-After")
	}
}

func TestCompanionValidation(t *testing.T) {
	router, _ := newAPI(t, testConfig())
	user := newTestUser(t, router)

	emptyBody := createConversationAPI(t, router, user.AccessToken, nil)
	if emptyBody.Status != http.StatusCreated {
		t.Fatalf("create without body = %d: %s", emptyBody.Status, emptyBody.Body)
	}
	conversation := decodeJSON[conversationEnvelope](t, emptyBody.Body).Data

	missingKey := sendMessageAPI(t, router, user.AccessToken, conversation.ID, "", "hello")
	if missingKey.Status != http.StatusBadRequest {
		t.Errorf("missing idempotency key = %d, want 400", missingKey.Status)
	}

	emptyContent := sendMessageAPI(t, router, user.AccessToken, conversation.ID, uuid.NewString(), "")
	if emptyContent.Status != http.StatusBadRequest {
		t.Errorf("empty content = %d, want 400", emptyContent.Status)
	}

	invalidConversation := sendMessageAPI(t, router, user.AccessToken, "not-a-uuid", uuid.NewString(), "hello")
	if invalidConversation.Status != http.StatusNotFound {
		t.Errorf("invalid conversation = %d, want 404", invalidConversation.Status)
	}
}
