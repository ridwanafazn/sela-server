//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
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

func TestPhase2SchemaConstraints(t *testing.T) {
	db, _ := testdb.New(t)
	ctx := context.Background()
	userID := insertUser(t, db, "phase2@example.com")

	conversationID := uuid.NewString()
	exec(t, db,
		`INSERT INTO conversations (id, user_id, title) VALUES ($1, $2, $3)`,
		conversationID, userID, "Conversation",
	)

	messageID := uuid.NewString()
	exec(t, db,
		`INSERT INTO messages (id, conversation_id, user_id, role, content, idempotency_key)
		 VALUES ($1, $2, $3, 'user', 'hello', $4)`,
		messageID, conversationID, userID, uuid.NewString(),
	)

	_, err := db.SQL.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, user_id, role, content) VALUES ($1, $2, $3, 'system', 'bad')`,
		uuid.NewString(), conversationID, userID,
	)
	requirePgError(t, err, "23514", "messages_role_check")

	// The same idempotency key is unique per conversation.
	key := uuid.NewString()
	exec(t, db,
		`INSERT INTO messages (id, conversation_id, user_id, role, content, idempotency_key)
		 VALUES ($1, $2, $3, 'user', 'first', $4)`,
		uuid.NewString(), conversationID, userID, key,
	)
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, user_id, role, content, idempotency_key)
		 VALUES ($1, $2, $3, 'user', 'second', $4)`,
		uuid.NewString(), conversationID, userID, key,
	)
	requirePgError(t, err, "23505", "messages_conversation_idempotency_key_key")

	journalID := uuid.NewString()
	exec(t, db,
		`INSERT INTO journals (id, user_id, content, entry_date) VALUES ($1, $2, $3, CURRENT_DATE)`,
		journalID, userID, "journal content",
	)

	embeddingID := uuid.NewString()
	exec(t, db,
		`INSERT INTO journal_embeddings
		 (id, journal_id, user_id, chunk_index, start_offset, end_offset, content_hash, embedding, embedding_model)
		 VALUES ($1, $2, $3, 0, 0, 10, 'hash', $4::vector, 'text-embedding-004')`,
		embeddingID, journalID, userID, vectorLiteral(768),
	)

	var dimensions int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT vector_dims(embedding) FROM journal_embeddings WHERE id = $1`, embeddingID,
	).Scan(&dimensions); err != nil {
		t.Fatalf("read vector dimensions: %v", err)
	}
	if dimensions != 768 {
		t.Errorf("vector dimensions = %d, want 768", dimensions)
	}

	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO journal_embeddings
		 (id, journal_id, user_id, chunk_index, start_offset, end_offset, content_hash, embedding, embedding_model)
		 VALUES ($1, $2, $3, 1, 0, 10, 'hash', '[0,0,0]'::vector, 'wrong')`,
		uuid.NewString(), journalID, userID,
	)
	if err == nil {
		t.Error("journal_embeddings accepted a vector with the wrong dimension")
	}

	memoryID := uuid.NewString()
	exec(t, db,
		`INSERT INTO personal_memories (id, user_id, content, confidence, source_count)
		 VALUES ($1, $2, 'The user prefers concise answers.', 0.9, 1)`,
		memoryID, userID,
	)

	exec(t, db,
		`INSERT INTO memory_sources (id, memory_id, journal_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), memoryID, journalID,
	)
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO memory_sources (id, memory_id, journal_id, message_id) VALUES ($1, $2, $3, $4)`,
		uuid.NewString(), memoryID, journalID, messageID,
	)
	requirePgError(t, err, "23514", "memory_sources_one_source_check")

	conflictingMemoryID := uuid.NewString()
	exec(t, db,
		`INSERT INTO personal_memories (id, user_id, content, confidence, source_count)
		 VALUES ($1, $2, 'The user dislikes concise answers.', 0.6, 0)`,
		conflictingMemoryID, userID,
	)
	exec(t, db,
		`INSERT INTO memory_conflicts (id, memory_id, conflicting_memory_id)
		 VALUES ($1, $2, $3)`,
		uuid.NewString(), memoryID, conflictingMemoryID,
	)
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO memory_conflicts (id, memory_id, conflicting_memory_id)
		 VALUES ($1, $2, $3)`,
		uuid.NewString(), memoryID, conflictingMemoryID,
	)
	requirePgError(t, err, "23505", "memory_conflicts_pair_key")
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO memory_conflicts (id, memory_id, conflicting_memory_id)
		 VALUES ($1, $2, $3)`,
		uuid.NewString(), memoryID, memoryID,
	)
	requirePgError(t, err, "23514", "memory_conflicts_distinct_check")
	assertIndexesExist(t, db, "memory_conflicts", []string{
		"memory_conflicts_pair_key",
		"memory_conflicts_conflicting_memory_id_idx",
	})

	insightID := uuid.NewString()
	exec(t, db,
		`INSERT INTO insights
		 (id, user_id, title, summary, observation, interpretation, confidence,
		  confidence_label, reflection_question, suggested_direction, generation_type)
		 VALUES ($1, $2, 'Title', 'Summary', 'Observation', 'Interpretation', 0.7,
		         'moderate', 'Does this fit?', 'improve', 'on_demand')`,
		insightID, userID,
	)

	exec(t, db,
		`INSERT INTO insight_evidence (id, insight_id, journal_id, note) VALUES ($1, $2, $3, 'evidence')`,
		uuid.NewString(), insightID, journalID,
	)
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO insight_evidence (id, insight_id, journal_id, message_id, note)
		 VALUES ($1, $2, $3, $4, 'two sources')`,
		uuid.NewString(), insightID, journalID, messageID,
	)
	requirePgError(t, err, "23514", "insight_evidence_one_source_check")

	exec(t, db,
		`INSERT INTO scheduled_jobs (id, user_id, job_type, payload, run_at)
		 VALUES ($1, $2, 'journal.embedding', '{}', now())`,
		uuid.NewString(), userID,
	)
	_, err = db.SQL.ExecContext(ctx,
		`INSERT INTO scheduled_jobs (id, user_id, job_type, run_at, attempts, max_attempts)
		 VALUES ($1, $2, 'journal.embedding', now(), 4, 3)`,
		uuid.NewString(), userID,
	)
	requirePgError(t, err, "23514", "scheduled_jobs_attempts_check")

	assertIndexesExist(t, db, "scheduled_jobs", []string{
		"scheduled_jobs_status_run_at_idx",
		"scheduled_jobs_user_id_job_type_idx",
		"scheduled_jobs_user_idempotency_key_idx",
		"scheduled_jobs_claimable_idx",
	})
	assertIndexesExist(t, db, "insights", []string{
		"insights_job_id_key",
		"insights_user_id_created_at_id_idx",
		"insights_user_id_period_start_idx",
	})
}

func vectorLiteral(dimension int) string {
	parts := make([]string, dimension)
	for index := range parts {
		parts[index] = "0"
	}
	return "[" + strings.Join(parts, ",") + "]"
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
