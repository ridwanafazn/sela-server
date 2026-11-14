package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/platform/health"
	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
)

type fakeReadiness struct {
	checks map[string]string
	err    error
	delay  time.Duration
}

func (f fakeReadiness) Ready(ctx context.Context) (map[string]string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.checks, f.err
}

type healthBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type errorDetail struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorObject struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []errorDetail `json:"details"`
	RequestID *string       `json:"request_id"`
}

type errorBody struct {
	Error errorObject `json:"error"`
}

func newRouter(t *testing.T, readiness health.Readiness, timeout time.Duration) *gin.Engine {
	t.Helper()

	return httpx.NewRouter(httpx.RouterOptions{
		Env:              config.EnvTest,
		Log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		Readiness:        readiness,
		ReadinessTimeout: timeout,
	})
}

func get(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestLive(t *testing.T) {
	router := newRouter(t, fakeReadiness{checks: map[string]string{}}, time.Second)
	recorder := get(t, router, "/api/v1/health/live")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var body healthBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Checks != nil {
		t.Errorf("liveness returned dependency checks: %v", body.Checks)
	}
}

func TestRequestIDHeaderIsValidUUID(t *testing.T) {
	router := newRouter(t, fakeReadiness{checks: map[string]string{}}, time.Second)
	recorder := get(t, router, "/api/v1/health/live")

	value := recorder.Header().Get("X-Request-ID")
	if value == "" {
		t.Fatal("X-Request-ID header is missing")
	}
	if _, err := uuid.Parse(value); err != nil {
		t.Errorf("X-Request-ID is not a UUID: %q", value)
	}
}

func TestReadyWhenDependenciesAreHealthy(t *testing.T) {
	router := newRouter(t, fakeReadiness{
		checks: map[string]string{"database": "ok", "migrations": "ok"},
	}, time.Second)

	recorder := get(t, router, "/api/v1/health/ready")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body healthBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Checks["database"] != "ok" || body.Checks["migrations"] != "ok" {
		t.Errorf("checks = %v, want database and migrations ok", body.Checks)
	}
}

func TestReadyFailsWithoutLeakingDependencyDetails(t *testing.T) {
	router := newRouter(t, fakeReadiness{
		checks: map[string]string{"database": "unavailable"},
		err:    errors.New("dial tcp secret-connection-string: connection refused"),
	}, time.Second)

	recorder := get(t, router, "/api/v1/health/ready")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if body := recorder.Body.String(); strings.Contains(body, "secret-connection-string") {
		t.Errorf("response leaked dependency details: %s", body)
	}

	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "UPSTREAM_UNAVAILABLE" {
		t.Errorf("error code = %q, want UPSTREAM_UNAVAILABLE", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Error("error message is empty")
	}
	if body.Error.Details == nil {
		t.Error("error details must be an empty array, not null")
	}
	if body.Error.RequestID == nil || *body.Error.RequestID == "" {
		t.Error("error request_id is missing")
	}
}

func TestReadyHonorsTimeout(t *testing.T) {
	router := newRouter(t, fakeReadiness{
		checks: map[string]string{"database": "ok"},
		delay:  500 * time.Millisecond,
	}, 25*time.Millisecond)

	start := time.Now()
	recorder := get(t, router, "/api/v1/health/ready")
	elapsed := time.Since(start)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if elapsed >= 400*time.Millisecond {
		t.Errorf("readiness took %s; timeout was not enforced", elapsed)
	}
}

func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	router := newRouter(t, fakeReadiness{checks: map[string]string{}}, time.Second)
	recorder := get(t, router, "/api/v1/does-not-exist")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Errorf("error code = %q, want NOT_FOUND", body.Error.Code)
	}
}
