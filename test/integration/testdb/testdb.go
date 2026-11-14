//go:build integration

// Package testdb provisions disposable PostgreSQL databases for integration
// tests. Every test gets its own database so tests can run in parallel.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/ridwanafazn/sela-server/internal/platform/database"
)

const defaultURL = "postgres://sela:sela@localhost:5432/sela_test?sslmode=disable"

var databaseCounter atomic.Uint64

// New creates a uniquely named database, applies every migration, and returns
// a pool connected to it plus its DSN. The database is dropped when the test
// finishes.
func New(t *testing.T) (*sql.DB, string) {
	t.Helper()

	base := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if base == "" {
		base = defaultURL
	}

	name := fmt.Sprintf("sela_test_%d_%d", time.Now().UTC().UnixNano(), databaseCounter.Add(1))
	if len(name) > 63 {
		t.Fatalf("generated database name %q exceeds PostgreSQL's 63 byte limit", name)
	}

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	admin.SetMaxOpenConns(1)
	admin.SetMaxIdleConns(1)

	t.Cleanup(func() {
		_ = admin.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database %s: %v", name, err)
	}

	db, err := sql.Open("pgx", withDatabase(t, base, name))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	dsn := withDatabase(t, base, name)

	t.Cleanup(func() {
		_ = db.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		if _, err := admin.ExecContext(dropCtx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to test database %s: %v", name, err)
	}

	if err := database.MigrateUp(dsn); err != nil {
		t.Fatalf("apply migrations to test database %s: %v", name, err)
	}

	return db, dsn
}

func withDatabase(t *testing.T, rawURL, databaseName string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not a valid URL: %v", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("TEST_DATABASE_URL must be a postgres:// URL, got %q", rawURL)
	}

	parsed.Path = "/" + databaseName
	return parsed.String()
}
