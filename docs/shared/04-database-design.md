# SELA Database Design

Version: 1.4

Status: P0 schema decisions locked, with 2026-11-21 product vision decisions

## 1. Database Strategy

SELA uses Neon PostgreSQL as its primary relational database.

pgvector is used for semantic retrieval.

PostgreSQL is the source of truth for:

- User accounts.
- Profiles.
- Authentication sessions.
- Journals.
- Conversations.
- Messages.
- Memories.
- Insights.
- Insight feedback.
- Background jobs.
- Idempotency records.

Embeddings and generated summaries are derived data.

The DBML file is a conceptual design artifact. SQL migrations are authoritative for the deployed database.

## 2. Design Principles

1. Every user-owned record must have an ownership relationship.
2. Original journal and message content must be preserved independently from AI-generated content.
3. Derived records must retain source references.
4. Sensitive data must not be copied unnecessarily.
5. Deletion must account for dependent records and derived data.
6. Schema changes must use versioned migrations.
7. Production must not depend on GORM AutoMigrate.
8. Constraints that protect invariants must live in the database, not only in application code.

## 3. Core Entities

### users

Stores authentication identity and account lifecycle.

Fields:

- id: UUID primary key.
- email: normalized, case-insensitive, unique.
- password_hash.
- status: `active`, `deletion_pending`, `deleted`.
- email_verified_at, nullable.
- deletion_requested_at, nullable.
- created_at.
- updated_at.
- deleted_at, nullable.

Rules:

- Email MUST be unique case-insensitively. Use `citext` or a unique index on `lower(email)`.
- Account deletion sets `status = deletion_pending` and schedules a purge job.
- A deleted account MUST NOT be able to authenticate.

### profiles

Stores user profile and communication preferences.

Fields:

- id.
- user_id, unique.
- display_name.
- language.
- timezone.
- communication_style.
- created_at.
- updated_at.

`communication_style` values:

- adaptive.
- direct.
- gentle.
- balanced.

Granular privacy preferences are deferred. The profile keeps only the preferences required for the MVP experience.

### auth_sessions

Stores refresh-session metadata and rotation state.

Fields:

- id.
- user_id.
- token_family_id.
- refresh_token_hash.
- expires_at.
- revoked_at, nullable.
- replaced_by, nullable.
- created_at.
- last_used_at, nullable.

Rules:

- Do not store raw refresh tokens.
- Refresh tokens rotate on use.
- Reuse of a rotated token revokes the entire token family.
- `replaced_by` points to the replacement session for audit and reuse detection.

### auth_rate_limits

Stores hashed subjects for durable rate limiting.

Fields:

- id.
- action.
- subject_hash.
- created_at.

Rules:

- Never store raw IP addresses or account identifiers.
- Rows may be removed after their rate-limit window expires.
- The lookup index is `(action, subject_hash, created_at DESC)`.

### journals

Stores original journal entries and drafts.

Fields:

- id.
- user_id.
- title, nullable.
- content.
- entry_date: date.
- status: `draft` or `active`.
- version: integer, starts at 1 and increments on every update.
- created_at.
- updated_at.
- deleted_at, nullable.

Rules:

- Journal content remains independent of AI analysis.
- Drafts are visible only to the owner and are excluded from AI retrieval, memory extraction, and insight generation.
- Only `active` entries are eligible for embeddings and personal memory extraction.
- `entry_date` defaults to the user's local date at creation and may be edited.
- `version` increments atomically on every update and is used by `If-Match`.
- Full-text search uses a generated `tsvector` column and a GIN index.
- Offline synchronization reads changes ordered by `updated_at` and `id`, including tombstones.

### journal_embeddings

Stores derived vector representations.

Fields:

- id.
- user_id.
- journal_id.
- chunk_index.
- start_offset.
- end_offset.
- content_hash.
- embedding: `vector(768)`.
- embedding_model.
- created_at.

Rules:

- Embeddings are derived data and may be regenerated.
- Do not duplicate `chunk_text` in this table. Store offsets and a content hash instead.
- A change to source content or embedding model requires regeneration.
- The vector dimension is fixed at 768 for the MVP embedding model. Changing it requires a migration and re-embedding.
- HNSW or IVFFlat is chosen after measuring the dataset. The index must match the selected distance metric.

### conversations

Stores Companion conversation sessions.

Fields:

- id.
- user_id.
- title, nullable.
- created_at.
- updated_at.
- deleted_at, nullable.

### messages

Stores conversation messages.

Fields:

- id.
- conversation_id.
- user_id.
- role: `user` or `assistant`.
- content.
- idempotency_key, nullable.
- metadata JSONB, nullable.
- created_at.

Rules:

- Do not store or expose internal system prompts.
- `idempotency_key` is unique per conversation to prevent duplicate user messages.
- `metadata` MAY contain approved provider usage metadata, never raw prompts or full private content.
- Messages are ordered by `created_at` and `id`.

### personal_memories

Stores structured personal context extracted from user records.

Fields:

- id.
- user_id.
- content.
- category, nullable.
- sensitivity: `normal` or `sensitive`.
- status: `candidate`, `active`, `rejected`, `superseded`, `deleted`.
- confidence, 0 to 1.
- source_count.
- supersedes_memory_id, nullable.
- last_confirmed_at, nullable.
- created_at.
- updated_at.
- deleted_at, nullable.

Rules:

- Memories are interpretations or structured summaries, not replacements for original journals.
- `candidate` means the memory has not been confirmed by the user.
- `active` means the memory is confirmed or otherwise eligible with normal priority.
- `rejected` memories are excluded from retrieval.
- `superseded` memories are retained for audit but excluded from retrieval.
  User-facing copy is "digantikan", not "archived".
- `deleted` memories are soft-deleted and excluded from default lists.
  User-facing copy is "dihapus".
- Conflicting records MUST NOT be silently merged.
- If all sources for a memory are deleted, set it to `candidate` with `source_count = 0` for review.
- `supersedes_memory_id` records replacement without destroying the old record.
- Sensitive content is skipped by default. It may be stored only with explicit
  user consent, minimal content, restricted retrieval, and a revocable consent
  record.
- Sensitive memories are excluded from Companion and Insight retrieval unless
  active consent exists.

### memory_consents

Stores explicit, revocable consent for scoped memory use.

Fields:

- id.
- user_id.
- scope: `sensitive_memory`.
- granted.
- granted_at, nullable.
- revoked_at, nullable.
- created_at.
- updated_at.

Rules:

- One row per user and scope.
- `granted = true` with `revoked_at IS NULL` means consent is active.
- Revoking consent preserves the historical grant time and sets `revoked_at`.
- Sensitive memories are excluded from retrieval when consent is absent or
  revoked.
- Account deletion cascades through `user_id`.

### memory_sources

Links memories to supporting records.

Fields:

- id.
- memory_id.
- journal_id, nullable.
- message_id, nullable.
- source_excerpt, optional and minimized.
- created_at.

Rules:

- Exactly one of `journal_id` or `message_id` MUST be present.
- Enforce with a PostgreSQL CHECK constraint.
- `source_excerpt` must be minimized and must not duplicate full source content.

### memory_conflicts

Records contradictions between two memories. Neither memory is merged or
deleted; both remain reviewable.

Fields:

- id.
- memory_id.
- conflicting_memory_id.
- created_at.

Rules:

- A memory cannot conflict with itself.
- Each ordered pair is unique.
- Relations are recorded in both directions for display.
- Deleting a memory cascades its conflict records.

### insights

Stores completed insight records.

Fields:

- id.
- user_id.
- job_id, nullable and unique when present.
- title.
- summary.
- observation.
- interpretation.
- hypothesis, nullable.
- confidence, 0 to 1.
- confidence_label: `strong`, `moderate`, `limited`.
- reflection_question.
- suggested_direction: `improve`, `maintain`, `accept`.
- period_start, nullable.
- period_end, nullable.
- generation_type: `on_demand`, `weekly`, `monthly`.
- model, nullable.
- prompt_version, nullable.
- created_at.
- updated_at.
- deleted_at, nullable.

Rules:

- An insight is created only when generation completes successfully with sufficient evidence.
- Pending, processing, and failed states belong to the job, not the insight.
- `observation`, `interpretation`, and `hypothesis` are separate fields.
- Insight content is immutable. User feedback is stored separately.
- If evidence is insufficient, do not create an insight row.
- `generation_type` supports `on_demand`, `weekly`, and `monthly`. Weekly and
  monthly scheduled insights are MVP.
- Scheduled insight generation must use the same evidence threshold and
  idempotency rules as on-demand generation.
- A user-configurable schedule preference record is required for scheduled
  insights. Its exact schema is an implementation decision pending design.

### insight_evidence

Links insights to supporting source records.

Fields:

- id.
- insight_id.
- journal_id, nullable.
- message_id, nullable.
- memory_id, nullable.
- note.
- source_deleted_at, nullable.
- created_at.

Rules:

- Exactly one of `journal_id`, `message_id`, or `memory_id` MUST be present.
- Enforce with a PostgreSQL CHECK constraint.
- Evidence MUST be validated for ownership.
- When a source is deleted, set `source_deleted_at` and redact content that could reveal the deleted source.
- An insight with no usable evidence should be reviewed and may need to be hidden or deleted.

### insight_feedback

Stores user feedback on insights.

Fields:

- id.
- insight_id.
- user_id.
- action: `confirm`, `correct`, `reject`.
- note, nullable.
- created_at.

Rules:

- Feedback is append-only for audit.
- The latest feedback is returned in the insight response.
- Feedback does not modify the AI-generated insight content.

### insight_schedules

Stores one weekly or monthly reflection schedule per user.

Fields:

- id.
- user_id, unique.
- cadence: `weekly` or `monthly`.
- timezone.
- enabled.
- next_run_at.
- last_run_at, nullable.
- created_at.
- updated_at.

Rules:

- One active schedule row per user.
- `next_run_at` is stored in UTC and computed from the profile timezone.
- The scheduler only selects `enabled = true` rows whose `next_run_at` is due.
- Fan-out is idempotent per user and period.
- The next run is advanced only after a job is enqueued.
- Account deletion cascades through `user_id`.

### ai_usage_events

Stores operational AI metadata for cost, latency, and error tracking.

Fields:

- id.
- user_id.
- operation: `companion`, `memory.extraction.journal`, `memory.extraction.message`, `insight.generate`, `embedding.journal`.
- provider.
- model.
- input_tokens, output_tokens.
- estimated_cost_usd.
- latency_ms.
- success.
- error_category, nullable.
- reference_id, nullable.
- created_at.

Rules:

- Never store prompts, source text, or generated content.
- Tokens, cost, and latency must be non-negative.
- Rows are user-owned and cascade on account deletion.
- Monthly budgets are derived from these events.

### scheduled_jobs

Stores durable asynchronous tasks.

Fields:

- id.
- user_id, nullable for system jobs.
- job_type.
- payload JSONB.
- status: `pending`, `processing`, `completed`, `retry`, `failed`.
- attempts.
- max_attempts.
- run_at.
- locked_at, nullable.
- locked_by, nullable.
- last_error, nullable.
- result JSONB, nullable; structured outcome for jobs such as insight generation.
- idempotency_key, nullable.
- created_at.
- updated_at.

Rules:

- Workers claim eligible jobs with a lock that expires.
- Attempts must not exceed `max_attempts`.
- A job with an idempotency key MUST NOT create duplicate side effects.
- Use a partial index for claimable jobs.

### idempotency_keys

Stores replayable responses for costly POST operations.

Fields:

- id.
- user_id.
- key.
- endpoint.
- request_hash.
- response_status, nullable.
- response_body JSONB, nullable.
- job_id, nullable.
- expires_at.
- created_at.

Rules:

- Unique on `(user_id, endpoint, key)`.
- Same key and same request hash returns the stored response.
- Same key and different request hash returns `409`.
- TTL is 24 hours.
- A cleanup job removes expired records.

## 4. Relationships

- One user has one profile.
- One user has many authentication sessions.
- One user has many journals.
- One journal has many derived embeddings.
- One user has many conversations.
- One conversation has many messages.
- One user has many memories.
- One memory has many memory sources.
- One memory may have many conflicting memories.
- One memory may supersede another memory.
- One user has many insights.
- One insight has many evidence records.
- One insight has many feedback records.
- One user may have many scheduled jobs.
- One user may have many idempotency records.

## 5. Data Integrity

Required constraints:

- Unique normalized email.
- Foreign keys for ownership relationships.
- Valid status values through CHECK constraints.
- Non-negative job attempts.
- `attempts <= max_attempts`.
- Confidence between 0 and 1.
- `period_start <= period_end`.
- Exactly one evidence source per evidence row.
- Exactly one source per memory-source row.
- A memory conflict cannot reference itself and each pair is unique.
- Unique idempotency keys per user.
- Unique message idempotency keys per conversation.

Use database constraints for invariants that must remain true regardless of application code.

## 6. Indexing Strategy

Recommended indexes:

- `users(email)` unique.
- `profiles(user_id)` unique.
- `auth_sessions(user_id, expires_at)`.
- `auth_sessions(token_family_id)`.
- `journals(user_id, entry_date DESC, id DESC)`.
- `journals(user_id, status, updated_at DESC)`.
- `journals(user_id, updated_at, id)` for offline synchronization.
- `auth_rate_limits(action, subject_hash, created_at DESC)`.
- `auth_rate_limits(created_at)`.
- Full-text search: generated `tsvector` column on journal title and content with GIN.
- `conversations(user_id, updated_at DESC)`.
- `messages(conversation_id, created_at, id)`.
- `messages(conversation_id, idempotency_key)` unique.
- `personal_memories(user_id, status, updated_at DESC)`.
- `personal_memories(user_id, category)`.
- `insights(user_id, created_at DESC, id DESC)`.
- `insights(user_id, period_start)`.
- `insights(job_id)` unique when `job_id` is present.
- `memory_sources(memory_id)`.
- `memory_conflicts(memory_id, conflicting_memory_id)` unique.
- `memory_conflicts(conflicting_memory_id)`.
- `insight_evidence(insight_id)`.
- `insight_feedback(insight_id, created_at)`.
- `scheduled_jobs(status, run_at)` partial index for claimable jobs.
- `ai_usage_events(user_id, created_at DESC)`.
- `ai_usage_events(operation, created_at DESC)`.
- `ai_usage_events(reference_id)` partial when present.
- `idempotency_keys(user_id, endpoint, key)` unique.
- `idempotency_keys(expires_at)`.

For vector search:

- Use `vector(768)`.
- Use cosine distance for the MVP embedding model unless measurement justifies another metric.
- Choose HNSW or IVFFlat after measuring the dataset and query behavior.
- Apply user and eligibility filters before returning retrieval results.
- Do not create a vector index until the embedding model and dimension are final.

## 7. JSONB Usage

JSONB is suitable for:

- Job payloads.
- Approved message metadata.
- Idempotency response bodies.

Do not place all relational data into JSONB.

Core entities and relationships remain relational.

## 8. Migration Strategy

Use a versioned migration tool.

Recommended: `golang-migrate`.

Rules:

- Migrations must be committed to version control.
- Migrations must be reviewed.
- Production migrations must run as a controlled release step.
- Avoid destructive schema changes without a migration plan.
- Prefer expand-and-contract changes for fields used by deployed clients.
- Enable `citext` and `vector` extensions before dependent migrations.
- Add CHECK constraints explicitly in SQL; DBML notes are not constraints.

Do not use GORM AutoMigrate in production.

## 9. Neon Connection Strategy

Use pooled connections for application traffic when compatible.

Use direct connections for migration or administrative tasks when needed.

Configure connection limits based on Cloud Run concurrency and instance scaling.

Avoid allowing autoscaling to create more database connections than Neon can safely handle.

## 10. Vector Data Lifecycle

Embeddings are derived from source content.

Store:

- Model identifier.
- Source ID.
- Chunk index.
- Start and end offsets.
- Source content hash.
- Creation timestamp.

When source content changes:

1. Mark the previous embedding stale.
2. Queue regeneration.
3. Generate a new embedding.
4. Update the active version.

When a source is deleted:

- Delete or invalidate its embeddings.
- Remove it from retrieval.
- Remove related memory sources.
- Re-evaluate memories that lose their last source.
- Mark insight evidence as `source_deleted_at` and redact sensitive notes.

Do not duplicate full source text in the embedding table.

## 11. Retention and Deletion

Account deletion must cover:

- User profile.
- Journals.
- Conversations.
- Messages.
- Memories.
- Insights.
- Insight feedback.
- Embeddings.
- Sessions.
- Idempotency records.
- User-owned jobs.
- AI usage events.

Deletion behavior:

- The account is deactivated immediately.
- A durable purge job removes user-owned data.
- Backups may retain copies for a limited period; disclose the retention policy.
- Deleted data must not be restored into active use without reconciliation.
- Journal deletion is soft delete by default; derived data is removed or invalidated.
- Memory source deletion may downgrade a memory to `candidate` with zero sources.
- Insight evidence referencing a deleted source is redacted and marked `source_deleted_at`.

## 12. Offline Journal Synchronization

The L1 offline design uses the server journal table as the synchronization source.

Requirements:

- `journals.version` starts at 1 and increments atomically on every update.
- Updates and deletes use `If-Match` with the last known version.
- A stale version returns `409 VERSION_CONFLICT`.
- `GET /journal-changes` reads journals ordered by `updated_at` and `id`.
- Deleted journals remain as tombstones with `deleted_at` set.
- The changes query includes tombstones and returns an opaque cursor.
- The client keeps an outbox and retries creates with an idempotency key.
- The server does not store the client outbox.
- AI processing occurs only after an active journal is synchronized.

The index `journals(user_id, updated_at, id)` supports incremental synchronization.

## 13. Deferred Schema Work

The following are deferred to the next development phase:

- Granular `user_preferences` and broader privacy-consent records.
- `journal_analyses` and retrieval of stored journal analysis.
- Data export records.
- Audit-log tables beyond insight feedback and idempotency.
- Row-level security evaluation.

Scheduled insight preferences and sensitive memory consent are implemented in
migration `17`.

## 14. Database ERD

The conceptual schema is defined in:

`database-erd.dbml`

The DBML is a design artifact. SQL migrations remain authoritative for deployed database structure.
