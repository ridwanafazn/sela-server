# Per-entry Journal Analysis Backend

Date: 2026-11-21
Status: Phase 4A backend implemented.

## 1. Scope

Per-entry journal analysis is the first Phase 4 post-MVP capability. It is tied
to one owned, published journal entry and is stored separately from the
original entry.

Implemented:

- asynchronous analysis requests,
- idempotent request replay,
- one stored analysis per journal version,
- latest-analysis inspection with a stale marker,
- analysis deletion with pending-job cancellation,
- job status polling,
- evidence and uncertainty fields,
- AI budget and rate limits,
- usage tracking without content,
- export and account-deletion participation.

Not implemented:

- mobile analysis UI,
- streaming or synchronous analysis,
- analysis history screens,
- premium entitlement enforcement,
- analysis of drafts, conversations, or multiple entries at once,
- push notification delivery.

## 2. API

Endpoints:

```http
POST   /api/v1/journals/{journalId}/analysis
GET    /api/v1/journals/{journalId}/analysis
DELETE /api/v1/journals/{journalId}/analysis
GET    /api/v1/journal-analysis-jobs/{jobId}
```

`POST` requires an `Idempotency-Key` UUID and returns `202`.

The analysis is only available for an owned journal with status `active`.
Drafts return `409`. A journal version can have only one stored analysis; a
second request returns `409` until the analysis is deleted or the journal is
updated.

`GET` returns the newest stored analysis. When the journal has been edited
since the analysis was generated, `stale` is `true` and
`current_journal_version` is newer than `journal_version`.

`DELETE` removes every stored analysis for the journal and marks pending or
retryable analysis jobs as failed. It never changes the journal entry.

## 3. Job model

Job type: `journal.analysis`.

Payload:

```json
{
  "journal_id": "uuid",
  "journal_version": 1
}
```

Outcomes:

- `analysis_created`,
- `insufficient_evidence`,
- `journal_changed`,
- `budget_exhausted`.

`journal_changed` is returned when the entry was deleted, unpublished, or
edited after the request. No analysis is stored for a stale version.

## 4. Data model

Migration `19` adds `journal_analyses`:

- `id`,
- `user_id`,
- `journal_id`,
- `journal_version`,
- `job_id`,
- `summary`,
- `observation`,
- `interpretation`,
- `hypothesis`,
- `confidence`,
- `confidence_label`,
- `reflection_question`,
- `suggested_direction`,
- `evidence_note`,
- `model`,
- `prompt_version`,
- `created_at`.

Rules:

- one stored analysis per `(journal_id, journal_version)`,
- one analysis per `job_id`,
- deleting a journal removes its stored analyses,
- account deletion cascades through foreign keys,
- deleting an analysis is a hard delete,
- the original journal is never modified.

The API returns evidence as a single journal source with its entry date and a
short evidence note.

## 5. Generation

Prompt: `prompts/journal_analysis/v1.md`.
Prompt version: `journal_analysis/v1`.

The pipeline:

1. Verifies job idempotency.
2. Loads the owned journal and checks version and publication state.
3. Rejects content shorter than 80 runes with `insufficient_evidence`.
4. Reuses an existing analysis for the same version if one exists.
5. Checks the user/global AI budget.
6. Loads the profile language.
7. Renders the prompt with the entry only.
8. Calls the configured language model with a JSON schema.
9. Validates fields, evidence note, direction, confidence, and length limits.
10. Returns `insufficient_evidence` when confidence is below `0.40`.
11. Stores the analysis separately and records usage metadata.

Logs contain job, analysis, journal, model, token counts, and outcomes. They
never contain journal or analysis text.

## 6. Rate Limits

New settings:

```text
RATE_LIMIT_ANALYSIS_PER_HOUR=10
RATE_LIMIT_ANALYSIS_PER_DAY=30
```

The budget check is repeated inside the worker so a queued job cannot bypass a
budget that was exhausted after the request.

## 7. Privacy

- Analysis uses only the requested entry.
- No memory, conversation, or cross-entry retrieval is performed.
- The analysis is stored separately from the journal.
- Users can inspect and delete it.
- Deleting the journal removes stored analyses; account deletion cascades.
- Export includes stored analyses and never includes provider secrets or auth
  token hashes.
- Analysis content is not used for training.

## 8. Verification

Unit coverage:

- output validation,
- low-confidence rejection,
- copied-evidence rejection,
- field and note bounds,
- prompt safety and schema requirements.

Integration coverage:

- async request and idempotent replay,
- job polling and completion,
- stored analysis and evidence response,
- usage event with analysis reference,
- duplicate-version conflict,
- draft rejection,
- cross-user isolation,
- journal update and stale marker,
- re-analysis after edit,
- deletion and pending-job cancellation,
- re-request after deletion.

Local staging verification with the real OpenRouter model
`deepseek/deepseek-v4.1-flash-1017`:

- analysis job completed with `analysis_created`,
- model and prompt version recorded,
- one evidence item returned,
- export included the stored analysis without credential hashes,
- deletion returned `204` and the next read returned `404`.

## 9. Phase 4 position

Phase 4A is complete on the backend. Other Phase 4 candidates still require
product evidence: privacy center, push notifications, encrypted read cache,
multi-device sync, advanced search, and cost/model optimization.
