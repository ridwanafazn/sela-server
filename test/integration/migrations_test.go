//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/migrations"
	"github.com/ridwanafazn/sela-server/test/integration/testdb"
)

func exec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()

	if _, err := db.SQL.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func requirePgError(t *testing.T, err error, code, constraint string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected PostgreSQL error %s on constraint %s, got nil", code, constraint)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != code {
		t.Errorf("SQLSTATE = %s, want %s (%v)", pgErr.Code, code, err)
	}
	if constraint != "" && pgErr.ConstraintName != constraint {
		t.Errorf("constraint = %q, want %q", pgErr.ConstraintName, constraint)
	}
}

func insertUser(t *testing.T, db *database.DB, email string) string {
	t.Helper()

	id := uuid.NewString()
	exec(t, db,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		id, email, "argon2id-placeholder",
	)
	return id
}

func TestMigrationsAreReversible(t *testing.T) {
	db, dsn := testdb.New(t)
	ctx := context.Background()

	latest, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}

	version, dirty, err := database.MigrationVersion(ctx, db.SQL)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version != latest || dirty {
		t.Fatalf("version = %d dirty = %t, want %d and false", version, dirty, latest)
	}

	if err := database.MigrateDown(dsn); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}

	version, dirty, err = database.MigrationVersion(ctx, db.SQL)
	if err != nil {
		t.Fatalf("MigrationVersion after down: %v", err)
	}
	if version != 0 || dirty {
		t.Fatalf("after down version = %d dirty = %t, want 0 and false", version, dirty)
	}

	if err := database.MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp after down: %v", err)
	}

	version, dirty, err = database.MigrationVersion(ctx, db.SQL)
	if err != nil {
		t.Fatalf("MigrationVersion after up: %v", err)
	}
	if version != latest || dirty {
		t.Fatalf("after up version = %d dirty = %t, want %d and false", version, dirty, latest)
	}
}

func TestUsersEmailIsCaseInsensitiveUnique(t *testing.T) {
	db, _ := testdb.New(t)

	insertUser(t, db, "Person@Example.com")
	_, err := db.SQL.ExecContext(context.Background(),
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		uuid.NewString(), "person@example.com", "argon2id-placeholder",
	)

	requirePgError(t, err, "23505", "users_email_key")
}

func TestUsersStatusConstraint(t *testing.T) {
	db, _ := testdb.New(t)

	_, err := db.SQL.ExecContext(context.Background(),
		`INSERT INTO users (id, email, password_hash, status) VALUES ($1, $2, $3, $4)`,
		uuid.NewString(), "status@example.com", "argon2id-placeholder", "banned",
	)

	requirePgError(t, err, "23514", "users_status_check")
}

func TestProfilesCommunicationStyleConstraint(t *testing.T) {
	db, _ := testdb.New(t)
	userID := insertUser(t, db, "profile@example.com")

	_, err := db.SQL.ExecContext(context.Background(),
		`INSERT INTO profiles (id, user_id, communication_style) VALUES ($1, $2, $3)`,
		uuid.NewString(), userID, "harsh",
	)

	requirePgError(t, err, "23514", "profiles_communication_style_check")
}

func TestJournalsVersionConstraint(t *testing.T) {
	db, _ := testdb.New(t)
	userID := insertUser(t, db, "journal-version@example.com")

	_, err := db.SQL.ExecContext(context.Background(),
		`INSERT INTO journals (id, user_id, content, entry_date, version) VALUES ($1, $2, $3, CURRENT_DATE, $4)`,
		uuid.NewString(), userID, "content", 0,
	)

	requirePgError(t, err, "23514", "journals_version_check")
}

func TestJournalsFullTextSearchAndIndexes(t *testing.T) {
	db, _ := testdb.New(t)
	ctx := context.Background()
	userID := insertUser(t, db, "journal-search@example.com")

	journalID := uuid.NewString()
	exec(t, db,
		`INSERT INTO journals (id, user_id, title, content, entry_date, status) VALUES ($1, $2, $3, $4, CURRENT_DATE, $5)`,
		journalID, userID, "Late night", "I could not sleep well and kept thinking about work.", "active",
	)

	var matches bool
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT search_vector @@ websearch_to_tsquery('simple', $1) FROM journals WHERE id = $2`,
		"sleep", journalID,
	).Scan(&matches); err != nil {
		t.Fatalf("query search vector: %v", err)
	}
	if !matches {
		t.Error("search_vector did not match journal content")
	}

	var draftMatches bool
	draftID := uuid.NewString()
	exec(t, db,
		`INSERT INTO journals (id, user_id, content, entry_date, status) VALUES ($1, $2, $3, CURRENT_DATE, $4)`,
		draftID, userID, "sleep notes in a draft", "draft",
	)
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT search_vector @@ websearch_to_tsquery('simple', $1) FROM journals WHERE id = $2`,
		"sleep", draftID,
	).Scan(&draftMatches); err != nil {
		t.Fatalf("query draft search vector: %v", err)
	}
	if !draftMatches {
		t.Error("search_vector was not generated for a draft; draft exclusion belongs to retrieval queries")
	}

	wantIndexes := []string{
		"journals_user_id_entry_date_id_idx",
		"journals_user_id_status_updated_at_idx",
		"journals_user_id_updated_at_id_idx",
		"journals_search_vector_idx",
	}
	assertIndexesExist(t, db, "journals", wantIndexes)
}

func TestAuthRateLimitTable(t *testing.T) {
	db, _ := testdb.New(t)

	exec(t, db,
		`INSERT INTO auth_rate_limits (id, action, subject_hash) VALUES ($1, $2, $3)`,
		uuid.NewString(), "auth.login.ip", "hash",
	)

	var count int
	if err := db.SQL.QueryRowContext(context.Background(),
		`SELECT count(*) FROM auth_rate_limits WHERE action = $1`, "auth.login.ip",
	).Scan(&count); err != nil {
		t.Fatalf("count rate limit rows: %v", err)
	}
	if count != 1 {
		t.Errorf("rate limit rows = %d, want 1", count)
	}

	assertIndexesExist(t, db, "auth_rate_limits", []string{
		"auth_rate_limits_lookup_idx",
		"auth_rate_limits_created_at_idx",
	})
}

func TestIdempotencyKeysAreUniquePerEndpoint(t *testing.T) {
	db, _ := testdb.New(t)
	userID := insertUser(t, db, "idempotency@example.com")
	expiresAt := time.Now().Add(24 * time.Hour)

	const insert = `INSERT INTO idempotency_keys (id, user_id, key, endpoint, request_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	exec(t, db, insert, uuid.NewString(), userID, "key-1", "POST /journals", "hash-a", expiresAt)

	_, err := db.SQL.ExecContext(context.Background(), insert,
		uuid.NewString(), userID, "key-1", "POST /journals", "hash-b", expiresAt,
	)
	requirePgError(t, err, "23505", "idempotency_keys_user_endpoint_key_key")

	exec(t, db, insert, uuid.NewString(), userID, "key-1", "POST /insights", "hash-b", expiresAt)
}

func TestAuthSessionsRefreshHashIsUnique(t *testing.T) {
	db, _ := testdb.New(t)
	userID := insertUser(t, db, "session@example.com")

	const insert = `INSERT INTO auth_sessions (id, user_id, token_family_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3, $4, now() + interval '30 days')`

	exec(t, db, insert, uuid.NewString(), userID, uuid.NewString(), "same-hash")

	_, err := db.SQL.ExecContext(context.Background(), insert,
		uuid.NewString(), userID, uuid.NewString(), "same-hash",
	)
	requirePgError(t, err, "23505", "auth_sessions_refresh_token_hash_key")
}

func TestUserDeletionCascades(t *testing.T) {
	db, _ := testdb.New(t)
	ctx := context.Background()
	userID := insertUser(t, db, "cascade@example.com")
	journalID := uuid.NewString()

	exec(t, db, `INSERT INTO profiles (id, user_id) VALUES ($1, $2)`, uuid.NewString(), userID)
	exec(t, db,
		`INSERT INTO auth_sessions (id, user_id, token_family_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, now() + interval '30 days')`,
		uuid.NewString(), userID, uuid.NewString(), "cascade-hash",
	)
	exec(t, db,
		`INSERT INTO journals (id, user_id, content, entry_date) VALUES ($1, $2, $3, CURRENT_DATE)`,
		journalID, userID, "content",
	)
	exec(t, db,
		`INSERT INTO idempotency_keys (id, user_id, key, endpoint, request_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5, now() + interval '24 hours')`,
		uuid.NewString(), userID, "cascade-key", "POST /journals", "hash",
	)

	exec(t, db, `DELETE FROM users WHERE id = $1`, userID)

	for _, table := range []string{"profiles", "auth_sessions", "journals", "idempotency_keys"} {
		var count int
		query := "SELECT count(*) FROM " + table + " WHERE user_id = $1"
		if err := db.SQL.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s still has %d rows for deleted user", table, count)
		}
	}
}

func TestDraftAndActiveStatusesAreAllowed(t *testing.T) {
	db, _ := testdb.New(t)
	userID := insertUser(t, db, "statuses@example.com")

	for _, status := range []string{"draft", "active"} {
		exec(t, db,
			`INSERT INTO journals (id, user_id, content, entry_date, status) VALUES ($1, $2, $3, CURRENT_DATE, $4)`,
			uuid.NewString(), userID, "content", status,
		)
	}
}

func assertIndexesExist(t *testing.T, db *database.DB, table string, want []string) {
	t.Helper()

	rows, err := db.SQL.QueryContext(context.Background(),
		`SELECT indexname FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1`, table,
	)
	if err != nil {
		t.Fatalf("query indexes for %s: %v", table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	found := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}

	for _, name := range want {
		if !found[name] {
			t.Errorf("missing index %s on %s", name, table)
		}
	}
}
