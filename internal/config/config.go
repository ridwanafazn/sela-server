// Package config loads and validates the backend configuration from the
// environment. Configuration is validated at startup so the process fails
// fast on missing or malformed values.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Supported application environments.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Config is the complete runtime configuration.
type Config struct {
	App       App
	HTTP      HTTP
	Database  Database
	Health    Health
	JWT       JWT
	RateLimit RateLimit
	AI        AI
	Log       Log
}

// App holds process-level settings.
type App struct {
	Env  string
	Name string
	Port int
}

// HTTP holds server timeouts and request limits.
type HTTP struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	MaxBodyBytes    int64
}

// Database holds connection pool settings. URL is the pooled connection used
// by the application. MigrationURL is the direct connection used by
// migrations; it falls back to URL when unset.
type Database struct {
	URL             string
	MigrationURL    string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

// Health holds the overall readiness check budget.
type Health struct {
	Timeout time.Duration
}

// JWT holds access-token settings. Refresh tokens are opaque and are not
// signed.
type JWT struct {
	AccessSecret string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	Issuer       string
	Audience     string
}

// RateLimit holds the DB-backed authentication limits.
type RateLimit struct {
	Secret              string
	TrustedProxyHops    int
	LoginPerMinuteIP    int
	LoginPerHourAccount int
	RegisterPerHourIP   int
	RefreshPerMinuteIP  int
}

// AI holds provider and model settings. Fake providers are the local default
// so the application can run without external credentials.
type AI struct {
	Provider             string
	BaseURL              string
	APIKey               string
	Model                string
	Timeout              time.Duration
	MaxRetries           int
	EmbeddingProvider    string
	EmbeddingBaseURL     string
	EmbeddingAPIKey      string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingTimeout     time.Duration
	VertexProjectID      string
	VertexRegion         string
	VertexEmbeddingModel string
	VertexBaseURL        string
}

// Log holds structured logging settings.
type Log struct {
	Level  string
	Format string
}

// Load reads configuration from the process environment. In non-production
// environments it also loads a local .env file when present. Existing
// environment variables always take precedence.
func Load() (*Config, error) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env != EnvProduction && env != EnvStaging {
		// Best effort: a missing .env file is expected in containers.
		_ = godotenv.Load()
	}

	return load(os.LookupEnv)
}

func load(lookup func(string) (string, bool)) (*Config, error) {
	var problems []string

	appEnv := stringValue(lookup, "APP_ENV", EnvDevelopment)
	if !isOneOf(appEnv, EnvDevelopment, EnvTest, EnvStaging, EnvProduction) {
		problems = append(problems, fmt.Sprintf("APP_ENV must be one of %q, %q, %q, %q", EnvDevelopment, EnvTest, EnvStaging, EnvProduction))
	}

	appName := stringValue(lookup, "APP_NAME", "sela-server")
	if appName == "" {
		problems = append(problems, "APP_NAME must not be empty")
	}

	appPort := intValue(lookup, "APP_PORT", 8080, &problems)
	if appPort < 1 || appPort > 65535 {
		problems = append(problems, "APP_PORT must be between 1 and 65535")
	}

	logLevel := strings.ToLower(stringValue(lookup, "LOG_LEVEL", "info"))
	if !isOneOf(logLevel, "debug", "info", "warn", "error") {
		problems = append(problems, "LOG_LEVEL must be one of debug, info, warn, error")
	}

	logFormat := strings.ToLower(stringValue(lookup, "LOG_FORMAT", "json"))
	if !isOneOf(logFormat, "json", "text") {
		problems = append(problems, "LOG_FORMAT must be one of json, text")
	}

	databaseURL := strings.TrimSpace(stringValue(lookup, "DATABASE_URL", ""))
	if databaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}

	migrationURL := strings.TrimSpace(stringValue(lookup, "DATABASE_URL_UNPOOLED", ""))
	if migrationURL == "" {
		migrationURL = databaseURL
	}

	httpCfg := HTTP{
		ReadTimeout:     durationValue(lookup, "HTTP_READ_TIMEOUT", 10*time.Second, &problems),
		WriteTimeout:    durationValue(lookup, "HTTP_WRITE_TIMEOUT", 30*time.Second, &problems),
		IdleTimeout:     durationValue(lookup, "HTTP_IDLE_TIMEOUT", 60*time.Second, &problems),
		ShutdownTimeout: durationValue(lookup, "HTTP_SHUTDOWN_TIMEOUT", 15*time.Second, &problems),
		MaxBodyBytes:    int64Value(lookup, "HTTP_MAX_BODY_BYTES", 1<<20, &problems),
	}
	if httpCfg.MaxBodyBytes < 1 {
		problems = append(problems, "HTTP_MAX_BODY_BYTES must be at least 1")
	}

	dbCfg := Database{
		URL:             databaseURL,
		MigrationURL:    migrationURL,
		MaxOpenConns:    intValue(lookup, "DATABASE_MAX_OPEN_CONNS", 10, &problems),
		MaxIdleConns:    intValue(lookup, "DATABASE_MAX_IDLE_CONNS", 5, &problems),
		ConnMaxLifetime: durationValue(lookup, "DATABASE_CONN_MAX_LIFETIME", 30*time.Minute, &problems),
		ConnMaxIdleTime: durationValue(lookup, "DATABASE_CONN_MAX_IDLE_TIME", 5*time.Minute, &problems),
		PingTimeout:     durationValue(lookup, "DATABASE_PING_TIMEOUT", 2*time.Second, &problems),
	}

	if dbCfg.MaxOpenConns < 1 {
		problems = append(problems, "DATABASE_MAX_OPEN_CONNS must be at least 1")
	}
	if dbCfg.MaxIdleConns < 0 {
		problems = append(problems, "DATABASE_MAX_IDLE_CONNS must not be negative")
	}
	if dbCfg.MaxIdleConns > dbCfg.MaxOpenConns {
		problems = append(problems, "DATABASE_MAX_IDLE_CONNS must not exceed DATABASE_MAX_OPEN_CONNS")
	}

	healthCfg := Health{
		Timeout: durationValue(lookup, "HEALTH_CHECK_TIMEOUT", 5*time.Second, &problems),
	}

	accessSecret := strings.TrimSpace(stringValue(lookup, "JWT_ACCESS_SECRET", ""))
	if accessSecret == "" {
		problems = append(problems, "JWT_ACCESS_SECRET is required")
	} else if len(accessSecret) < 32 {
		problems = append(problems, "JWT_ACCESS_SECRET must be at least 32 characters")
	}

	jwtCfg := JWT{
		AccessSecret: accessSecret,
		AccessTTL:    durationValue(lookup, "JWT_ACCESS_TTL", 15*time.Minute, &problems),
		RefreshTTL:   durationValue(lookup, "JWT_REFRESH_TTL", 720*time.Hour, &problems),
		Issuer:       stringValue(lookup, "JWT_ISSUER", "sela-api"),
		Audience:     stringValue(lookup, "JWT_AUDIENCE", "sela-mobile"),
	}
	if jwtCfg.Issuer == "" {
		problems = append(problems, "JWT_ISSUER must not be empty")
	}
	if jwtCfg.Audience == "" {
		problems = append(problems, "JWT_AUDIENCE must not be empty")
	}
	if jwtCfg.RefreshTTL <= jwtCfg.AccessTTL {
		problems = append(problems, "JWT_REFRESH_TTL must be greater than JWT_ACCESS_TTL")
	}

	rateLimitSecret := strings.TrimSpace(stringValue(lookup, "RATE_LIMIT_SECRET", ""))
	if rateLimitSecret == "" {
		rateLimitSecret = accessSecret
	} else if len(rateLimitSecret) < 32 {
		problems = append(problems, "RATE_LIMIT_SECRET must be at least 32 characters")
	}

	rateLimitCfg := RateLimit{
		Secret:              rateLimitSecret,
		TrustedProxyHops:    intValue(lookup, "RATE_LIMIT_TRUSTED_PROXY_HOPS", 0, &problems),
		LoginPerMinuteIP:    intValue(lookup, "RATE_LIMIT_LOGIN_PER_MINUTE_IP", 5, &problems),
		LoginPerHourAccount: intValue(lookup, "RATE_LIMIT_LOGIN_PER_HOUR_ACCOUNT", 10, &problems),
		RegisterPerHourIP:   intValue(lookup, "RATE_LIMIT_REGISTER_PER_HOUR_IP", 3, &problems),
		RefreshPerMinuteIP:  intValue(lookup, "RATE_LIMIT_REFRESH_PER_MINUTE_IP", 30, &problems),
	}
	if rateLimitCfg.TrustedProxyHops < 0 || rateLimitCfg.TrustedProxyHops > 10 {
		problems = append(problems, "RATE_LIMIT_TRUSTED_PROXY_HOPS must be between 0 and 10")
	}
	for key, value := range map[string]int{
		"RATE_LIMIT_LOGIN_PER_MINUTE_IP":    rateLimitCfg.LoginPerMinuteIP,
		"RATE_LIMIT_LOGIN_PER_HOUR_ACCOUNT": rateLimitCfg.LoginPerHourAccount,
		"RATE_LIMIT_REGISTER_PER_HOUR_IP":   rateLimitCfg.RegisterPerHourIP,
		"RATE_LIMIT_REFRESH_PER_MINUTE_IP":  rateLimitCfg.RefreshPerMinuteIP,
	} {
		if value < 1 {
			problems = append(problems, key+" must be at least 1")
		}
	}

	aiCfg := AI{
		Provider:             strings.ToLower(stringValue(lookup, "AI_PROVIDER", "fake")),
		BaseURL:              strings.TrimSpace(stringValue(lookup, "AI_BASE_URL", "")),
		APIKey:               strings.TrimSpace(stringValue(lookup, "AI_API_KEY", "")),
		Model:                stringValue(lookup, "AI_MODEL", ""),
		Timeout:              durationValue(lookup, "AI_TIMEOUT", 60*time.Second, &problems),
		MaxRetries:           intValue(lookup, "AI_MAX_RETRIES", 3, &problems),
		EmbeddingProvider:    strings.ToLower(stringValue(lookup, "EMBEDDING_PROVIDER", "fake")),
		EmbeddingBaseURL:     strings.TrimSpace(stringValue(lookup, "EMBEDDING_BASE_URL", "")),
		EmbeddingAPIKey:      strings.TrimSpace(stringValue(lookup, "EMBEDDING_API_KEY", "")),
		EmbeddingModel:       stringValue(lookup, "EMBEDDING_MODEL", ""),
		EmbeddingDimension:   intValue(lookup, "EMBEDDING_DIMENSION", 768, &problems),
		EmbeddingTimeout:     durationValue(lookup, "EMBEDDING_TIMEOUT", 30*time.Second, &problems),
		VertexProjectID:      strings.TrimSpace(stringValue(lookup, "VERTEX_PROJECT_ID", "")),
		VertexRegion:         strings.TrimSpace(stringValue(lookup, "VERTEX_REGION", "asia-southeast1")),
		VertexEmbeddingModel: stringValue(lookup, "VERTEX_EMBEDDING_MODEL", "text-embedding-004"),
		VertexBaseURL:        strings.TrimSpace(stringValue(lookup, "VERTEX_BASE_URL", "")),
	}

	if !isOneOf(aiCfg.Provider, "fake", "openai") {
		problems = append(problems, "AI_PROVIDER must be fake or openai")
	}
	if aiCfg.Provider == "openai" {
		if aiCfg.BaseURL == "" {
			problems = append(problems, "AI_BASE_URL is required when AI_PROVIDER=openai")
		}
		if aiCfg.APIKey == "" {
			problems = append(problems, "AI_API_KEY is required when AI_PROVIDER=openai")
		}
		if aiCfg.Model == "" {
			problems = append(problems, "AI_MODEL is required when AI_PROVIDER=openai")
		}
	}
	if aiCfg.MaxRetries < 0 || aiCfg.MaxRetries > 5 {
		problems = append(problems, "AI_MAX_RETRIES must be between 0 and 5")
	}

	if aiCfg.EmbeddingDimension != 768 {
		problems = append(problems, "EMBEDDING_DIMENSION must be 768 for the MVP schema")
	}
	if !isOneOf(aiCfg.EmbeddingProvider, "fake", "openai", "vertex") {
		problems = append(problems, "EMBEDDING_PROVIDER must be fake, openai, or vertex")
	}
	switch aiCfg.EmbeddingProvider {
	case "openai":
		if aiCfg.EmbeddingBaseURL == "" {
			problems = append(problems, "EMBEDDING_BASE_URL is required when EMBEDDING_PROVIDER=openai")
		}
		if aiCfg.EmbeddingAPIKey == "" {
			problems = append(problems, "EMBEDDING_API_KEY is required when EMBEDDING_PROVIDER=openai")
		}
		if aiCfg.EmbeddingModel == "" {
			problems = append(problems, "EMBEDDING_MODEL is required when EMBEDDING_PROVIDER=openai")
		}
	case "vertex":
		if aiCfg.VertexProjectID == "" {
			problems = append(problems, "VERTEX_PROJECT_ID is required when EMBEDDING_PROVIDER=vertex")
		}
		if aiCfg.VertexRegion == "" {
			problems = append(problems, "VERTEX_REGION is required when EMBEDDING_PROVIDER=vertex")
		}
		if aiCfg.VertexEmbeddingModel == "" {
			problems = append(problems, "VERTEX_EMBEDDING_MODEL is required when EMBEDDING_PROVIDER=vertex")
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}

	return &Config{
		App: App{
			Env:  appEnv,
			Name: appName,
			Port: appPort,
		},
		HTTP:      httpCfg,
		Database:  dbCfg,
		Health:    healthCfg,
		JWT:       jwtCfg,
		RateLimit: rateLimitCfg,
		AI:        aiCfg,
		Log: Log{
			Level:  logLevel,
			Format: logFormat,
		},
	}, nil
}

func stringValue(lookup func(string) (string, bool), key, fallback string) string {
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func intValue(lookup func(string) (string, bool), key string, fallback int, problems *[]string) int {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}

	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer", key))
		return fallback
	}

	return value
}

func int64Value(lookup func(string) (string, bool), key string, fallback int64, problems *[]string) int64 {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}

	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer", key))
		return fallback
	}

	return value
}

func durationValue(lookup func(string) (string, bool), key string, fallback time.Duration, problems *[]string) time.Duration {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}

	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be a valid duration", key))
		return fallback
	}
	if value <= 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be greater than zero", key))
	}

	return value
}

func isOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
