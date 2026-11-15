package config

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func validValues() map[string]string {
	return map[string]string{
		"DATABASE_URL":      "postgres://user:pass@localhost:5432/sela?sslmode=disable",
		"JWT_ACCESS_SECRET": "test-only-access-secret-0123456789abcdef",
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(lookupFrom(validValues()))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.App.Env != EnvDevelopment || cfg.App.Name != "sela-server" || cfg.App.Port != 8080 {
		t.Errorf("app defaults = %+v", cfg.App)
	}
	if cfg.Database.MaxOpenConns != 10 || cfg.Database.MaxIdleConns != 5 {
		t.Errorf("pool defaults = (%d, %d), want (10, 5)", cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("ConnMaxLifetime = %s, want 30m", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Health.Timeout != 5*time.Second {
		t.Errorf("Health.Timeout = %s, want 5s", cfg.Health.Timeout)
	}
	if cfg.HTTP.MaxBodyBytes != 1<<20 {
		t.Errorf("HTTP.MaxBodyBytes = %d, want %d", cfg.HTTP.MaxBodyBytes, 1<<20)
	}
	if cfg.JWT.AccessTTL != 15*time.Minute || cfg.JWT.RefreshTTL != 720*time.Hour {
		t.Errorf("JWT TTLs = (%s, %s), want (15m, 720h)", cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	}
	if cfg.JWT.Issuer != "sela-api" || cfg.JWT.Audience != "sela-mobile" {
		t.Errorf("JWT issuer/audience = (%q, %q)", cfg.JWT.Issuer, cfg.JWT.Audience)
	}
	if cfg.RateLimit.Secret != cfg.JWT.AccessSecret {
		t.Error("RATE_LIMIT_SECRET did not fall back to JWT_ACCESS_SECRET")
	}
	if cfg.RateLimit.LoginPerMinuteIP != 5 || cfg.RateLimit.LoginPerHourAccount != 10 {
		t.Errorf("login limits = (%d, %d), want (5, 10)", cfg.RateLimit.LoginPerMinuteIP, cfg.RateLimit.LoginPerHourAccount)
	}
	if cfg.RateLimit.RegisterPerHourIP != 3 || cfg.RateLimit.RefreshPerMinuteIP != 30 {
		t.Errorf("register/refresh limits = (%d, %d), want (3, 30)", cfg.RateLimit.RegisterPerHourIP, cfg.RateLimit.RefreshPerMinuteIP)
	}
	if cfg.RateLimit.CompanionPerMinute != 30 {
		t.Errorf("CompanionPerMinute = %d, want 30", cfg.RateLimit.CompanionPerMinute)
	}
	if cfg.RateLimit.JournalPerMinute != 60 {
		t.Errorf("JournalPerMinute = %d, want 60", cfg.RateLimit.JournalPerMinute)
	}
	if cfg.RateLimit.InsightPerHour != 5 || cfg.RateLimit.InsightPerDay != 20 {
		t.Errorf("insight limits = (%d, %d), want (5, 20)", cfg.RateLimit.InsightPerHour, cfg.RateLimit.InsightPerDay)
	}
	if cfg.RateLimit.AccountDeletePerHour != 5 {
		t.Errorf("AccountDeletePerHour = %d, want 5", cfg.RateLimit.AccountDeletePerHour)
	}
	if cfg.AI.Provider != "fake" || cfg.AI.Model != "" {
		t.Errorf("AI defaults = (%q, %q), want (fake, empty model)", cfg.AI.Provider, cfg.AI.Model)
	}
	if cfg.AI.EmbeddingProvider != "fake" || cfg.AI.EmbeddingDimension != 768 {
		t.Errorf("embedding defaults = (%q, %d), want (fake, 768)", cfg.AI.EmbeddingProvider, cfg.AI.EmbeddingDimension)
	}
	if cfg.AI.Timeout != 60*time.Second || cfg.AI.EmbeddingTimeout != 30*time.Second {
		t.Errorf("AI timeouts = (%s, %s), want (60s, 30s)", cfg.AI.Timeout, cfg.AI.EmbeddingTimeout)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "json" {
		t.Errorf("log defaults = (%q, %q), want (info, json)", cfg.Log.Level, cfg.Log.Format)
	}
}

func TestLoadMigrationURLFallback(t *testing.T) {
	values := validValues()
	const databaseURL = "postgres://user:pass@localhost:5432/sela?sslmode=disable"

	cfg, err := load(lookupFrom(values))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Database.MigrationURL != databaseURL {
		t.Errorf("MigrationURL = %q, want fallback to DATABASE_URL", cfg.Database.MigrationURL)
	}

	const directURL = "postgres://user:pass@localhost:5432/sela?sslmode=disable&direct=true"
	values["DATABASE_URL_UNPOOLED"] = directURL
	cfg, err = load(lookupFrom(values))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Database.MigrationURL != directURL {
		t.Errorf("MigrationURL = %q, want %q", cfg.Database.MigrationURL, directURL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	values := validValues()
	delete(values, "DATABASE_URL")

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want DATABASE_URL failure")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error = %q, want it to mention DATABASE_URL", err)
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	values := validValues()
	delete(values, "JWT_ACCESS_SECRET")

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want JWT_ACCESS_SECRET failure")
	}
	if !strings.Contains(err.Error(), "JWT_ACCESS_SECRET") {
		t.Errorf("error = %q, want it to mention JWT_ACCESS_SECRET", err)
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	values := validValues()
	values["JWT_ACCESS_SECRET"] = "too-short"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want secret length failure")
	}
	if !strings.Contains(err.Error(), "at least 32") {
		t.Errorf("error = %q, want it to mention the minimum length", err)
	}
}

func TestLoadRejectsInvalidTokenLifetimes(t *testing.T) {
	values := validValues()
	values["JWT_ACCESS_TTL"] = "720h"
	values["JWT_REFRESH_TTL"] = "15m"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want TTL ordering failure")
	}
	if !strings.Contains(err.Error(), "JWT_REFRESH_TTL") {
		t.Errorf("error = %q, want it to mention JWT_REFRESH_TTL", err)
	}
}

func TestLoadReportsAllProblemsWithoutValues(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{
		"APP_ENV":                        "staging-ish",
		"APP_PORT":                       "not-a-port",
		"LOG_LEVEL":                      "verbose",
		"LOG_FORMAT":                     "xml",
		"DATABASE_MAX_OPEN_CONNS":        "2",
		"DATABASE_MAX_IDLE_CONNS":        "4",
		"HTTP_READ_TIMEOUT":              "-1s",
		"DATABASE_CONN_MAX_LIFETIME":     "soon",
		"RATE_LIMIT_LOGIN_PER_MINUTE_IP": "0",
	}))
	if err == nil {
		t.Fatal("load returned nil error, want validation failure")
	}

	message := err.Error()
	for _, want := range []string{
		"APP_ENV",
		"APP_PORT",
		"LOG_LEVEL",
		"LOG_FORMAT",
		"DATABASE_URL",
		"JWT_ACCESS_SECRET",
		"DATABASE_MAX_IDLE_CONNS",
		"HTTP_READ_TIMEOUT",
		"DATABASE_CONN_MAX_LIFETIME",
		"RATE_LIMIT_LOGIN_PER_MINUTE_IP",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %s", message, want)
		}
	}
}

func TestLoadRejectsInvalidPrecedence(t *testing.T) {
	values := validValues()
	values["DATABASE_MAX_OPEN_CONNS"] = "1"
	values["DATABASE_MAX_IDLE_CONNS"] = "2"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want idle > open failure")
	}
	if !strings.Contains(err.Error(), "DATABASE_MAX_IDLE_CONNS") {
		t.Errorf("error = %q, want it to mention DATABASE_MAX_IDLE_CONNS", err)
	}
}

func TestLoadRequiresOpenAIProviderSettings(t *testing.T) {
	values := validValues()
	values["AI_PROVIDER"] = "openai"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want missing OpenAI settings failure")
	}
	for _, want := range []string{"AI_BASE_URL", "AI_API_KEY", "AI_MODEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %s", err, want)
		}
	}
}

func TestLoadRequiresVertexEmbeddingSettings(t *testing.T) {
	values := validValues()
	values["EMBEDDING_PROVIDER"] = "vertex"
	delete(values, "VERTEX_PROJECT_ID")

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want missing Vertex settings failure")
	}
	if !strings.Contains(err.Error(), "VERTEX_PROJECT_ID") {
		t.Errorf("error = %q, want it to mention VERTEX_PROJECT_ID", err)
	}
}

func TestLoadRejectsWrongEmbeddingDimension(t *testing.T) {
	values := validValues()
	values["EMBEDDING_DIMENSION"] = "1536"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want dimension failure")
	}
	if !strings.Contains(err.Error(), "EMBEDDING_DIMENSION") {
		t.Errorf("error = %q, want it to mention EMBEDDING_DIMENSION", err)
	}
}

func TestLoadRejectsInvalidAIProvider(t *testing.T) {
	values := validValues()
	values["AI_PROVIDER"] = "unknown"

	_, err := load(lookupFrom(values))
	if err == nil {
		t.Fatal("load returned nil error, want provider failure")
	}
	if !strings.Contains(err.Error(), "AI_PROVIDER") {
		t.Errorf("error = %q, want it to mention AI_PROVIDER", err)
	}
}
