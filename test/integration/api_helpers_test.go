//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/app"
	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/test/integration/testdb"
)

const (
	testJWTSecret = "integration-test-secret-0123456789abcdef"
	testPassword  = "correct horse battery staple"
)

type errorDetail struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorPayload struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []errorDetail `json:"details"`
	RequestID *string       `json:"request_id"`
}

type errorEnvelope struct {
	Error errorPayload `json:"error"`
}

type authPayload struct {
	User struct {
		ID        string    `json:"id"`
		Email     string    `json:"email"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type authEnvelope struct {
	Data  authPayload  `json:"data"`
	Error errorPayload `json:"error"`
}

type mePayload struct {
	User struct {
		ID              string     `json:"id"`
		Email           string     `json:"email"`
		EmailVerified   bool       `json:"email_verified"`
		EmailVerifiedAt *time.Time `json:"email_verified_at"`
		CreatedAt       time.Time  `json:"created_at"`
	} `json:"user"`
	Profile struct {
		DisplayName        *string `json:"display_name"`
		Language           string  `json:"language"`
		Timezone           string  `json:"timezone"`
		CommunicationStyle string  `json:"communication_style"`
	} `json:"profile"`
}

type meEnvelope struct {
	Data  mePayload    `json:"data"`
	Error errorPayload `json:"error"`
}

type apiResult struct {
	Status int
	Header http.Header
	Body   []byte
}

type requestOptions struct {
	body    any
	token   string
	ip      string
	raw     string
	noJSON  bool
	headers map[string]string
}

func testConfig() *config.Config {
	return &config.Config{
		App: config.App{Env: config.EnvTest, Name: "sela-server", Port: 0},
		HTTP: config.HTTP{
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			IdleTimeout:     10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
			MaxBodyBytes:    1 << 20,
		},
		Database: config.Database{
			MaxOpenConns:    4,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
			PingTimeout:     2 * time.Second,
		},
		Health: config.Health{Timeout: 2 * time.Second},
		JWT: config.JWT{
			AccessSecret: testJWTSecret,
			AccessTTL:    15 * time.Minute,
			RefreshTTL:   720 * time.Hour,
			Issuer:       "sela-api",
			Audience:     "sela-mobile",
		},
		RateLimit: config.RateLimit{
			Secret:                       testJWTSecret,
			TrustedProxyHops:             0,
			LoginPerMinuteIP:             1000,
			LoginPerHourAccount:          1000,
			RegisterPerHourIP:            1000,
			RefreshPerMinuteIP:           1000,
			JournalPerMinute:             1000,
			CompanionPerMinute:           1000,
			InsightPerHour:               1000,
			InsightPerDay:                1000,
			AccountDeletePerHour:         1000,
			PasswordResetPerHourIP:       1000,
			PasswordResetPerHourAccount:  1000,
			EmailVerificationPerHourUser: 1000,
			ExportPerHourUser:            1000,
			AnalysisPerHour:              1000,
			AnalysisPerDay:               1000,
		},
		AI: config.AI{
			Provider:           "fake",
			Model:              "fake-chat",
			Timeout:            60 * time.Second,
			MaxRetries:         3,
			EmbeddingProvider:  "fake",
			EmbeddingModel:     "fake-embedding-768",
			EmbeddingDimension: 768,
			EmbeddingTimeout:   30 * time.Second,
		},
		Email: config.Email{
			Provider:  "noop",
			OutboxDir: "var/mail",
		},
		PrePublic: config.PrePublic{
			RequireEmailVerification: false,
			TermsVersion:             "2026-11-21",
			PrivacyVersion:           "2026-11-21",
		},
		Log: config.Log{Level: "error", Format: "json"},
	}
}

func newAPI(t *testing.T, cfg *config.Config) (*gin.Engine, *database.DB) {
	t.Helper()

	db, _ := testdb.New(t)
	router, err := app.BuildRouter(cfg, testLogger(), db)
	if err != nil {
		t.Fatalf("BuildRouter: %v", err)
	}

	return router, db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func do(t *testing.T, router *gin.Engine, method, path string, opts requestOptions) apiResult {
	t.Helper()

	var body io.Reader
	if opts.raw != "" {
		body = bytes.NewReader([]byte(opts.raw))
	} else if opts.body != nil {
		encoded, err := json.Marshal(opts.body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		body = bytes.NewReader(encoded)
	}

	request := httptest.NewRequest(method, path, body)
	if !opts.noJSON && (opts.body != nil || opts.raw != "") {
		request.Header.Set("Content-Type", "application/json")
	}
	if opts.token != "" {
		request.Header.Set("Authorization", "Bearer "+opts.token)
	}
	for key, value := range opts.headers {
		request.Header.Set(key, value)
	}
	if opts.ip != "" {
		request.RemoteAddr = opts.ip + ":12345"
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	validateResponseContract(t, request, recorder)

	return apiResult{
		Status: recorder.Code,
		Header: recorder.Header(),
		Body:   recorder.Body.Bytes(),
	}
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()

	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	return value
}

func register(t *testing.T, router *gin.Engine, email, password, displayName, ip string) apiResult {
	t.Helper()

	body := map[string]any{"email": email, "password": password}
	if displayName != "" {
		body["display_name"] = displayName
	}
	return do(t, router, http.MethodPost, "/api/v1/auth/register", requestOptions{body: body, ip: ip})
}

func login(t *testing.T, router *gin.Engine, email, password, ip string) apiResult {
	t.Helper()

	return do(t, router, http.MethodPost, "/api/v1/auth/login", requestOptions{
		body: map[string]any{"email": email, "password": password},
		ip:   ip,
	})
}

func refresh(t *testing.T, router *gin.Engine, token, ip string) apiResult {
	t.Helper()

	return do(t, router, http.MethodPost, "/api/v1/auth/refresh", requestOptions{
		body: map[string]any{"refresh_token": token},
		ip:   ip,
	})
}
