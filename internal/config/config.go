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
	App      App
	HTTP     HTTP
	Database Database
	Health   Health
	Log      Log
}

// App holds process-level settings.
type App struct {
	Env  string
	Name string
	Port int
}

// HTTP holds server timeouts and shutdown behavior.
type HTTP struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
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

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}

	return &Config{
		App: App{
			Env:  appEnv,
			Name: appName,
			Port: appPort,
		},
		HTTP:     httpCfg,
		Database: dbCfg,
		Health:   healthCfg,
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
