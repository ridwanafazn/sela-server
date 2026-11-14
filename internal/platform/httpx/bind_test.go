package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
)

type testRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type envelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func newBindRouter(limit int64) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(httpx.BodyLimit(limit))
	router.POST("/bind", func(c *gin.Context) {
		var request testRequest
		if !httpx.BindJSON(c, &request) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return router
}

func doRequest(router *gin.Engine, body, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/bind", strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestBindJSONAcceptsValidBody(t *testing.T) {
	recorder := doRequest(
		newBindRouter(1024),
		`{"email":"user@example.com","password":"long-enough"}`,
		"application/json",
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestBindJSONRejectsUnsupportedMediaType(t *testing.T) {
	recorder := doRequest(newBindRouter(1024), `{"email":"user@example.com"}`, "text/plain")

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "UNSUPPORTED_MEDIA_TYPE") {
		t.Errorf("body = %s, want UNSUPPORTED_MEDIA_TYPE", recorder.Body.String())
	}
}

func TestBindJSONReturnsFieldDetails(t *testing.T) {
	recorder := doRequest(newBindRouter(1024), `{"email":"not-an-email","password":"short"}`, "application/json")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var body envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want VALIDATION_ERROR", body.Error.Code)
	}
	if body.Error.Details == nil || len(body.Error.Details) != 2 {
		t.Fatalf("details = %+v, want two field errors", body.Error.Details)
	}

	fields := map[string]bool{}
	for _, detail := range body.Error.Details {
		fields[detail.Field] = true
	}
	if !fields["email"] || !fields["password"] {
		t.Errorf("fields = %v, want email and password", fields)
	}
}

func TestBindJSONRejectsMalformedJSON(t *testing.T) {
	recorder := doRequest(newBindRouter(1024), `{"email":`, "application/json")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
		t.Errorf("body = %s, want VALIDATION_ERROR", recorder.Body.String())
	}
}

func TestBindJSONRejectsEmptyBody(t *testing.T) {
	recorder := doRequest(newBindRouter(1024), "", "application/json")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestBodyLimitRejectsOversizedRequest(t *testing.T) {
	recorder := doRequest(
		newBindRouter(32),
		`{"email":"user@example.com","password":"long-enough"}`,
		"application/json",
	)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Errorf("body = %s, want PAYLOAD_TOO_LARGE", recorder.Body.String())
	}
}
