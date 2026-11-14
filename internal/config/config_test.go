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

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(lookupFrom(map[string]string{
		"DATABASE_URL": "postgres://user:pass@localhost:5432/sela?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.App.Env != EnvDevelopment {
		t.Errorf("App.Env = %q, want %q", cfg.App.Env, EnvDevelopment)
	}
	if cfg.App.Name != "sela-server" {
		t.Errorf("App.Name = %q, want %q", cfg.App.Name, "sela-server")
	}
	if cfg.App.Port != 8080 {
		t.Errorf("App.Port = %d, want 8080", cfg.App.Port)
	}
	if cfg.Database.MaxOpenConns != 10 || cfg.Database.MaxIdleConns != 5 {
		t.Errorf("pool defaults = (%d, %d), want (10, 5)", cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("ConnMaxLifetime = %s, want 30m", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Database.PingTimeout != 2*time.Second {
		t.Errorf("PingTimeout = %s, want 2s", cfg.Database.PingTimeout)
	}
	if cfg.Health.Timeout != 5*time.Second {
		t.Errorf("Health.Timeout = %s, want 5s", cfg.Health.Timeout)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "json" {
		t.Errorf("log defaults = (%q, %q), want (info, json)", cfg.Log.Level, cfg.Log.Format)
	}
}

func TestLoadMigrationURLFallback(t *testing.T) {
	const databaseURL = "postgres://user:pass@localhost:5432/sela?sslmode=disable"

	cfg, err := load(lookupFrom(map[string]string{"DATABASE_URL": databaseURL}))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Database.MigrationURL != databaseURL {
		t.Errorf("MigrationURL = %q, want fallback to DATABASE_URL", cfg.Database.MigrationURL)
	}

	const directURL = "postgres://user:pass@localhost:5432/sela?sslmode=disable&direct=true"
	cfg, err = load(lookupFrom(map[string]string{
		"DATABASE_URL":          databaseURL,
		"DATABASE_URL_UNPOOLED": directURL,
	}))
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Database.MigrationURL != directURL {
		t.Errorf("MigrationURL = %q, want %q", cfg.Database.MigrationURL, directURL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{}))
	if err == nil {
		t.Fatal("load returned nil error, want DATABASE_URL failure")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error = %q, want it to mention DATABASE_URL", err)
	}
}

func TestLoadReportsAllProblemsWithoutValues(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{
		"APP_ENV":                    "staging-ish",
		"APP_PORT":                   "not-a-port",
		"LOG_LEVEL":                  "verbose",
		"LOG_FORMAT":                 "xml",
		"DATABASE_MAX_OPEN_CONNS":    "2",
		"DATABASE_MAX_IDLE_CONNS":    "4",
		"HTTP_READ_TIMEOUT":          "-1s",
		"DATABASE_CONN_MAX_LIFETIME": "soon",
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
		"DATABASE_MAX_IDLE_CONNS",
		"HTTP_READ_TIMEOUT",
		"DATABASE_CONN_MAX_LIFETIME",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %s", message, want)
		}
	}
}

func TestLoadRejectsInvalidPrecedence(t *testing.T) {
	_, err := load(lookupFrom(map[string]string{
		"DATABASE_URL":            "postgres://localhost/sela",
		"DATABASE_MAX_OPEN_CONNS": "1",
		"DATABASE_MAX_IDLE_CONNS": "2",
	}))
	if err == nil {
		t.Fatal("load returned nil error, want idle > open failure")
	}
	if !strings.Contains(err.Error(), "DATABASE_MAX_IDLE_CONNS") {
		t.Errorf("error = %q, want it to mention DATABASE_MAX_IDLE_CONNS", err)
	}
}
