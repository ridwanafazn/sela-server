// Package config loads and validates the backend configuration from the
// environment. Configuration is validated at startup so the process fails
// fast on missing or malformed values.
package config

import (
	"fmt"
	"math"
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

// RequiredStagingModel is the only text model allowed for staging and
// production when the OpenAI-compatible provider is used.
const RequiredStagingModel = "deepseek/deepseek-v4.1-flash-1017"

// Config is the complete runtime configuration.
type Config struct {
	App       App
	HTTP      HTTP
	Database  Database
	Health    Health
	JWT       JWT
	RateLimit RateLimit
	AI        AI
	Safety    Safety
	Email     Email
	PrePublic PrePublic
	Log       Log
}

// App holds process-level settings.
type App struct {
	Env  string
	Name string
	Port int
}

// Email holds the outbound mail provider configuration.
type Email struct {
	Provider     string
	OutboxDir    string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
}

// PrePublic holds account gate and legal-version settings.
type PrePublic struct {
	RequireEmailVerification bool
	TermsVersion             string
	PrivacyVersion           string
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
	Secret                       string
	TrustedProxyHops             int
	LoginPerMinuteIP             int
	LoginPerHourAccount          int
	RegisterPerHourIP            int
	RefreshPerMinuteIP           int
	JournalPerMinute             int
	CompanionPerMinute           int
	InsightPerHour               int
	InsightPerDay                int
	AccountDeletePerHour         int
	PasswordResetPerHourIP       int
	PasswordResetPerHourAccount  int
	EmailVerificationPerHourUser int
	ExportPerHourUser            int
}

// AI holds provider, model, pricing, and budget settings. Fake providers are
// the local default so the application can run without external credentials.
type AI struct {
	Provider                string
	BaseURL                 string
	APIKey                  string
	Model                   string
	Timeout                 time.Duration
	MaxRetries              int
	EmbeddingProvider       string
	EmbeddingBaseURL        string
	EmbeddingAPIKey         string
	EmbeddingModel          string
	EmbeddingDimension      int
	EmbeddingTimeout        time.Duration
	VertexProjectID         string
	VertexRegion            string
	VertexEmbeddingModel    string
	VertexBaseURL           string
	InputCostPerMillion     float64
	OutputCostPerMillion    float64
	EmbeddingCostPerMillion float64
	UserMonthlyBudgetUSD    float64
	GlobalMonthlyBudgetUSD  float64
}

// Safety holds crisis resource configuration.
type Safety struct {
	ResourcesFile string
}

// EmbeddingModelName returns the effective embedding model identifier.
func (a AI) EmbeddingModelName() string {
	switch a.EmbeddingProvider {
	case "vertex":
		return a.VertexEmbeddingModel
	case "fake":
		if a.EmbeddingModel == "" {
			return "fake-embedding-768"
		}
		return a.EmbeddingModel
	default:
		return a.EmbeddingModel
	}
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
		WriteTimeout:    durationValue(lookup, "HTTP_WRITE_TIMEOUT", 180*time.Second, &problems),
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
		Secret:                       rateLimitSecret,
		TrustedProxyHops:             intValue(lookup, "RATE_LIMIT_TRUSTED_PROXY_HOPS", 0, &problems),
		LoginPerMinuteIP:             intValue(lookup, "RATE_LIMIT_LOGIN_PER_MINUTE_IP", 5, &problems),
		LoginPerHourAccount:          intValue(lookup, "RATE_LIMIT_LOGIN_PER_HOUR_ACCOUNT", 10, &problems),
		RegisterPerHourIP:            intValue(lookup, "RATE_LIMIT_REGISTER_PER_HOUR_IP", 3, &problems),
		RefreshPerMinuteIP:           intValue(lookup, "RATE_LIMIT_REFRESH_PER_MINUTE_IP", 30, &problems),
		JournalPerMinute:             intValue(lookup, "RATE_LIMIT_JOURNAL_PER_MINUTE", 60, &problems),
		CompanionPerMinute:           intValue(lookup, "RATE_LIMIT_COMPANION_PER_MINUTE", 30, &problems),
		InsightPerHour:               intValue(lookup, "RATE_LIMIT_INSIGHT_PER_HOUR", 5, &problems),
		InsightPerDay:                intValue(lookup, "RATE_LIMIT_INSIGHT_PER_DAY", 20, &problems),
		AccountDeletePerHour:         intValue(lookup, "RATE_LIMIT_ACCOUNT_DELETE_PER_HOUR", 5, &problems),
		PasswordResetPerHourIP:       intValue(lookup, "RATE_LIMIT_PASSWORD_RESET_PER_HOUR_IP", 5, &problems),
		PasswordResetPerHourAccount:  intValue(lookup, "RATE_LIMIT_PASSWORD_RESET_PER_HOUR_ACCOUNT", 5, &problems),
		EmailVerificationPerHourUser: intValue(lookup, "RATE_LIMIT_EMAIL_VERIFICATION_PER_HOUR_USER", 5, &problems),
		ExportPerHourUser:            intValue(lookup, "RATE_LIMIT_EXPORT_PER_HOUR_USER", 3, &problems),
	}
	if rateLimitCfg.TrustedProxyHops < 0 || rateLimitCfg.TrustedProxyHops > 10 {
		problems = append(problems, "RATE_LIMIT_TRUSTED_PROXY_HOPS must be between 0 and 10")
	}
	for key, value := range map[string]int{
		"RATE_LIMIT_LOGIN_PER_MINUTE_IP":              rateLimitCfg.LoginPerMinuteIP,
		"RATE_LIMIT_LOGIN_PER_HOUR_ACCOUNT":           rateLimitCfg.LoginPerHourAccount,
		"RATE_LIMIT_REGISTER_PER_HOUR_IP":             rateLimitCfg.RegisterPerHourIP,
		"RATE_LIMIT_REFRESH_PER_MINUTE_IP":            rateLimitCfg.RefreshPerMinuteIP,
		"RATE_LIMIT_JOURNAL_PER_MINUTE":               rateLimitCfg.JournalPerMinute,
		"RATE_LIMIT_COMPANION_PER_MINUTE":             rateLimitCfg.CompanionPerMinute,
		"RATE_LIMIT_INSIGHT_PER_HOUR":                 rateLimitCfg.InsightPerHour,
		"RATE_LIMIT_INSIGHT_PER_DAY":                  rateLimitCfg.InsightPerDay,
		"RATE_LIMIT_ACCOUNT_DELETE_PER_HOUR":          rateLimitCfg.AccountDeletePerHour,
		"RATE_LIMIT_PASSWORD_RESET_PER_HOUR_IP":       rateLimitCfg.PasswordResetPerHourIP,
		"RATE_LIMIT_PASSWORD_RESET_PER_HOUR_ACCOUNT":  rateLimitCfg.PasswordResetPerHourAccount,
		"RATE_LIMIT_EMAIL_VERIFICATION_PER_HOUR_USER": rateLimitCfg.EmailVerificationPerHourUser,
		"RATE_LIMIT_EXPORT_PER_HOUR_USER":             rateLimitCfg.ExportPerHourUser,
	} {
		if value < 1 {
			problems = append(problems, key+" must be at least 1")
		}
	}

	aiCfg := AI{
		Provider:                strings.ToLower(stringValue(lookup, "AI_PROVIDER", "fake")),
		BaseURL:                 strings.TrimSpace(stringValue(lookup, "AI_BASE_URL", "")),
		APIKey:                  strings.TrimSpace(stringValue(lookup, "AI_API_KEY", "")),
		Model:                   stringValue(lookup, "AI_MODEL", ""),
		Timeout:                 durationValue(lookup, "AI_TIMEOUT", 60*time.Second, &problems),
		MaxRetries:              intValue(lookup, "AI_MAX_RETRIES", 3, &problems),
		EmbeddingProvider:       strings.ToLower(stringValue(lookup, "EMBEDDING_PROVIDER", "fake")),
		EmbeddingBaseURL:        strings.TrimSpace(stringValue(lookup, "EMBEDDING_BASE_URL", "")),
		EmbeddingAPIKey:         strings.TrimSpace(stringValue(lookup, "EMBEDDING_API_KEY", "")),
		EmbeddingModel:          stringValue(lookup, "EMBEDDING_MODEL", ""),
		EmbeddingDimension:      intValue(lookup, "EMBEDDING_DIMENSION", 768, &problems),
		EmbeddingTimeout:        durationValue(lookup, "EMBEDDING_TIMEOUT", 30*time.Second, &problems),
		VertexProjectID:         strings.TrimSpace(stringValue(lookup, "VERTEX_PROJECT_ID", "")),
		VertexRegion:            strings.TrimSpace(stringValue(lookup, "VERTEX_REGION", "asia-southeast1")),
		VertexEmbeddingModel:    stringValue(lookup, "VERTEX_EMBEDDING_MODEL", "gemini-embedding-001"),
		VertexBaseURL:           strings.TrimSpace(stringValue(lookup, "VERTEX_BASE_URL", "")),
		InputCostPerMillion:     floatValue(lookup, "AI_INPUT_COST_PER_MILLION", 0, &problems),
		OutputCostPerMillion:    floatValue(lookup, "AI_OUTPUT_COST_PER_MILLION", 0, &problems),
		EmbeddingCostPerMillion: floatValue(lookup, "AI_EMBEDDING_COST_PER_MILLION", 0, &problems),
		UserMonthlyBudgetUSD:    floatValue(lookup, "AI_USER_MONTHLY_BUDGET_USD", 0, &problems),
		GlobalMonthlyBudgetUSD:  floatValue(lookup, "AI_GLOBAL_MONTHLY_BUDGET_USD", 0, &problems),
	}
	safetyCfg := Safety{
		ResourcesFile: strings.TrimSpace(stringValue(lookup, "SAFETY_RESOURCES_FILE", "")),
	}
	emailCfg := Email{
		Provider:     strings.ToLower(stringValue(lookup, "EMAIL_PROVIDER", "noop")),
		OutboxDir:    strings.TrimSpace(stringValue(lookup, "EMAIL_OUTBOX_DIR", "var/mail")),
		SMTPHost:     strings.TrimSpace(stringValue(lookup, "SMTP_HOST", "")),
		SMTPPort:     intValue(lookup, "SMTP_PORT", 587, &problems),
		SMTPUsername: strings.TrimSpace(stringValue(lookup, "SMTP_USERNAME", "")),
		SMTPPassword: strings.TrimSpace(stringValue(lookup, "SMTP_PASSWORD", "")),
		SMTPFrom:     strings.TrimSpace(stringValue(lookup, "SMTP_FROM", "")),
	}
	if !isOneOf(emailCfg.Provider, "noop", "file", "smtp") {
		problems = append(problems, "EMAIL_PROVIDER must be noop, file, or smtp")
	}
	if emailCfg.Provider == "file" && emailCfg.OutboxDir == "" {
		problems = append(problems, "EMAIL_OUTBOX_DIR is required when EMAIL_PROVIDER=file")
	}
	if emailCfg.Provider == "smtp" {
		if emailCfg.SMTPHost == "" {
			problems = append(problems, "SMTP_HOST is required when EMAIL_PROVIDER=smtp")
		}
		if emailCfg.SMTPPort < 1 || emailCfg.SMTPPort > 65535 {
			problems = append(problems, "SMTP_PORT must be between 1 and 65535")
		}
		if emailCfg.SMTPFrom == "" {
			problems = append(problems, "SMTP_FROM is required when EMAIL_PROVIDER=smtp")
		}
	}
	prePublicCfg := PrePublic{
		RequireEmailVerification: boolValue(lookup, "REQUIRE_EMAIL_VERIFICATION", false, &problems),
		TermsVersion:             stringValue(lookup, "TERMS_VERSION", "2026-11-21"),
		PrivacyVersion:           stringValue(lookup, "PRIVACY_VERSION", "2026-11-21"),
	}
	if prePublicCfg.TermsVersion == "" {
		problems = append(problems, "TERMS_VERSION must not be empty")
	}
	if prePublicCfg.PrivacyVersion == "" {
		problems = append(problems, "PRIVACY_VERSION must not be empty")
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
	if isOneOf(appEnv, EnvStaging, EnvProduction) &&
		aiCfg.Provider == "openai" &&
		aiCfg.Model != RequiredStagingModel {
		problems = append(problems,
			"AI_MODEL must be "+RequiredStagingModel+" in staging and production")
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

	for key, value := range map[string]float64{
		"AI_INPUT_COST_PER_MILLION":     aiCfg.InputCostPerMillion,
		"AI_OUTPUT_COST_PER_MILLION":    aiCfg.OutputCostPerMillion,
		"AI_EMBEDDING_COST_PER_MILLION": aiCfg.EmbeddingCostPerMillion,
		"AI_USER_MONTHLY_BUDGET_USD":    aiCfg.UserMonthlyBudgetUSD,
		"AI_GLOBAL_MONTHLY_BUDGET_USD":  aiCfg.GlobalMonthlyBudgetUSD,
	} {
		if value < 0 {
			problems = append(problems, key+" must not be negative")
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
		Safety:    safetyCfg,
		Email:     emailCfg,
		PrePublic: prePublicCfg,
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

func floatValue(lookup func(string) (string, bool), key string, fallback float64, problems *[]string) float64 {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		*problems = append(*problems, key+" must be a number")
		return fallback
	}
	return parsed
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

func boolValue(lookup func(string) (string, bool), key string, fallback bool, problems *[]string) bool {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		*problems = append(*problems, key+" must be true or false")
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
