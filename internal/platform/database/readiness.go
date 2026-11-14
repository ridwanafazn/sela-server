package database

import (
	"context"
	"fmt"
	"time"
)

// Readiness checks that the database is reachable and that the schema is at
// least at the version required by this binary.
type Readiness struct {
	db       *DB
	expected uint
	timeout  time.Duration
}

// NewReadiness creates a readiness checker.
func NewReadiness(db *DB, expectedVersion uint, timeout time.Duration) *Readiness {
	return &Readiness{db: db, expected: expectedVersion, timeout: timeout}
}

// Ready returns per-dependency check results. A non-nil error means the
// service must not receive traffic.
func (r *Readiness) Ready(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	checks := make(map[string]string, 2)

	if err := r.db.Ping(ctx); err != nil {
		checks["database"] = "unavailable"
		return checks, fmt.Errorf("database: %w", err)
	}
	checks["database"] = "ok"

	version, dirty, err := MigrationVersion(ctx, r.db.SQL)
	if err != nil {
		checks["migrations"] = "unavailable"
		return checks, fmt.Errorf("migrations: %w", err)
	}
	if dirty {
		checks["migrations"] = "dirty"
		return checks, fmt.Errorf("migrations: database is dirty at version %d", version)
	}
	if version < r.expected {
		checks["migrations"] = "outdated"
		return checks, fmt.Errorf("migrations: database version %d is behind required version %d", version, r.expected)
	}
	checks["migrations"] = "ok"

	return checks, nil
}
