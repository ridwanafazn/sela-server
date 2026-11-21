# Changelog

All notable backend changes are recorded here.

## Unreleased — B1 backend addendum — 2026-11-21

### Added

- Migration version `17` with `insight_schedules`, `memory_consents`, and
  `personal_memories.sensitivity`.
- `GET`, `PUT`, and `DELETE /insight-schedule`.
- Weekly and monthly fan-out through `sela-worker -schedules`.
- `GET` and `PUT /memory-consent`.
- Sensitive memory extraction only with active consent.
- Sensitive memory metadata and exclusion from Companion and Insight retrieval
  without consent.
- Active Companion and Insight prompt v2.
- OpenAPI `1.4.0` contract for schedule and consent operations.
- Staging preflight script and staging validation report.
- Neon staging schema migrated to version `17`.
- Local API/worker smoke test against Neon staging with fake providers.
- Non-GCP local staging path: Docker API/worker/scheduler, Ollama embeddings,
  OpenRouter DeepSeek, and ngrok.
- Periodic scheduler command and container for non-GCP hosts.
- Real-provider local staging verification: 768-dimensional Ollama embeddings,
  DeepSeek Companion and insight generation, schedule/consent lifecycle, account
  purge, and ngrok health check.
- Local beta-hardening operations:
  - `cmd/ops health` health gates,
  - `cmd/ops usage` AI usage and cost report,
  - `cmd/ops stale-memories` review count,
  - PostgreSQL 17 backup and restore verification,
  - incident runbook and security checklist.
- Offline sync backend:
  - `GET /journal-changes`,
  - `updated_at, id` cursor pagination,
  - tombstones,
  - draft inclusion,
  - `sync.server_time`,
  - cross-user isolation and integration coverage.

### Changed

- Weekly and monthly scheduled insights are promoted to MVP.
- Per-entry journal analysis is clarified as a core capability with post-MVP
  delivery.
- Frameworks and Life Priority are documented as internal guidance, not
  user-facing labels or scores.
- Companion and insight prompt v2 add healthy challenge, behavioral adaptive
  style, framework guidance, and Life Priority guidance.
- Insight feedback remains confirm/correct/reject; "explore further" is handled
  through Companion.
- Sensitive memory requires explicit, revocable consent; the default remains
  skip.
- Fixed technical decisions are recorded: HS256 + rotating refresh,
  asynchronous `202` + job polling, and Vertex `gemini-embedding-001` at 768
  dimensions.
- Memory vocabulary is clarified: internal `superseded`/`deleted`; user-facing
  "digantikan"/"dihapus"; no "archived" without an archive feature.

### Documentation

- Added `docs/shared/10-product-vision-decisions.md`.
- Updated the product specification, frameworks, architecture, database design,
  AI system design, API contract, security/privacy, operations, roadmap,
  backend AI integration, testing, freeze addendum, focus phase, README, and
  handoff.

### Verification

- `go test ./...`.
- `go test -tags=integration ./...` against PostgreSQL with pgvector.
- OpenAPI lint valid.
- New unit and integration coverage for schedule lifecycle, fan-out
  idempotency, weekly generation, consent grant/revoke, sensitive extraction,
  and retrieval exclusion.
- Neon staging smoke test with schedule and consent lifecycle, worker
  processing, account purge, and log redaction.

### Blocked

- Cloud Run and Cloud Scheduler deployment require an active billing account
  and enabled GCP APIs.
- Vertex and OpenRouter validation require `VERTEX_PROJECT_ID` and
  `AI_API_KEY`.

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
