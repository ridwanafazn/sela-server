# Changelog

All notable backend changes are recorded here.

## v0.2.1 — Vertex embedding compatibility — 2026-11-15

### Fixed

- Vertex embedding requests now send `parameters.outputDimensionality` and
  `autoTruncate`, allowing `gemini-embedding-001` to produce the locked
  768-dimensional vectors.
- The default Vertex embedding model is now `gemini-embedding-001`.

## v0.2.0 — Phase 2 backend frozen — 2026-11-15

Phase 2 backend is complete and frozen. The mobile client and staging
deployment remain separate deliverables.

### Added

- Authentication with Argon2id, HS256 access tokens, rotating refresh tokens,
  session revocation, and DB-backed auth rate limits.
- Profile and communication preferences with language, timezone, display name,
  and communication style.
- Journal CRUD with server-side drafts, full-text search, keyset pagination,
  optimistic concurrency, idempotent creates, and per-user write rate limits.
- Companion conversations with message history, idempotent sends, rate limits,
  versioned prompts, journal and memory retrieval context, and AI usage
  metadata.
- Deterministic crisis and elevated-distress safety handling with configurable
  locale resources. Crisis messages bypass the model and are not used for
  memory extraction.
- Embedding pipeline with content chunking, offsets, content hashes, durable
  jobs, worker processing, model isolation, and backfill by embedding model.
- Hybrid retrieval using PostgreSQL full-text search and pgvector with
  ownership filters, recency scoring, and text-only fallback.
- Memory extraction from journals and user messages with provenance validation,
  duplicate merging, review endpoints, conflict persistence, and supersede on
  explicit correction.
- Asynchronous insight generation with job polling, evidence validation,
  confidence and source thresholds, immutable insight content, and append-only
  feedback.
- Re-authenticated account deletion with immediate deactivation, session
  revocation, durable purge, and cascading deletion of user-owned data.
- AI usage tracking in `ai_usage_events` with tokens, latency, success, error
  category, estimated cost, and optional reference IDs.
- Per-user and global monthly AI budgets with request blocking and background
  extraction skipping.
- Versioned deterministic evaluation gates for safety and prompt injection.
- OpenAPI 1.3.0 contract updates for journal write 429 responses, per-operation
  413/415 responses, and memory conflicts.
- Database migrations through version 16.

### Operational notes

- Run migrations before starting the API or worker.
- The API and worker are separate processes.
- Run `make backfill` after changing the embedding model or memory extraction
  prompt version.
- Configure `SAFETY_RESOURCES_FILE` with locally reviewed crisis resources
  before production.
- Configure AI pricing and budgets before production cost enforcement.
- `GET /journal-changes`, encrypted offline storage, and conflict-resolution UI
  remain Phase 3 work.

### Verification

- `go test ./...`
- `go test -tags=integration ./...`
- `make check`
- `make eval`
- `govulncheck ./...`
- `docker build`
- Real E2E: journal -> memory -> Companion -> insight -> usage -> account purge.
