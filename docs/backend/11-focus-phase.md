# Focus Phase — Pre-Beta

- Starts after: backend `v0.2.0`
- Goal: move from a frozen local backend to a real-provider staging system and
  an online-first mobile MVP.
- Selected architecture: Cloud Run (API) + Neon (PostgreSQL/pgvector) + Vertex
  AI (`gemini-embedding-001`, 768 dimensions) + OpenRouter
  (`deepseek/deepseek-v4.1-flash-1017`).
- Target device: Samsung Galaxy A35, Android.
- Deployment guide: `docs/backend/12-cloud-run-neon-vertex-setup.md`.
- This phase does not start until the required inputs below are available.

## Session status at handoff

The B1 backend addendum is implemented and tested locally. Staging deployment is
the next action. See `HANDOFF.md`.

The pre-beta sequence is updated by the 2026-11-21 product vision decisions:

1. Real Vertex embeddings and live model evaluation.
2. Scheduled insights MVP addendum.
3. Cloud Run + Neon staging.
4. Android Flutter MVP against staging.
5. Beta hardening.
6. Offline-first journaling.

Journal analysis remains a core capability with post-MVP delivery. It is not in
the pre-beta sequence.

## MVP scope addendum — 2026-11-21

The following items are now MVP and are not part of the frozen `v0.2.0` backend:

- Weekly and monthly scheduled insights with user opt-in. **Backend implemented.**
- User-configurable, timezone-aware schedule preferences. **Backend implemented.**
- Sensitive memory storage with explicit, revocable consent. **Backend implemented.**
- Companion prompt v2 for healthy challenge, adaptive style, and framework
  guidance. **Active; deterministic gates pass.**
- Insight prompt v2 for framework and Life Priority guidance. **Active;
  deterministic gates pass.**
- Mobile memory review, insight review, scheduled insight controls, and
  editable profile preferences. **Pending mobile work.**

The decisions are recorded in
`docs/shared/10-product-vision-decisions.md`.

## Ordered workstreams

### 1. Real embeddings and live evaluation

Status: in progress. Prompt v2 is active; real Vertex credentials and live model
evaluation are pending.

Tasks:

- Use the fixed embedding decision: Vertex AI `gemini-embedding-001` at 768
  dimensions. Reopening this requires a new decision record.
- Configure the embedding provider and privacy settings.
- Run `make backfill`.
- Verify vector retrieval against real embeddings.
- Run `make eval`.
- Run a live evaluation with the production model for:
  - Companion relevance,
  - insight evidence accuracy,
  - memory extraction precision,
  - tone and safety,
  - prompt injection,
  - healthy challenge without shaming,
  - adaptive style behavior,
  - framework relevance and user burden,
  - latency and cost.
- Run the live evaluation against the active `prompts/companion/v2.md` and
  `prompts/insight/v2.md`.
- Verify that sensitive memory remains skipped by default and that no sensitive
  content is retrieved without active consent.

Exit gate:

- Retrieval works with real vectors.
- Evaluation report is documented.
- Prompt v2 live quality is reviewed.
- No unapproved provider receives private content.
- Sensitive memory consent policy is verified.

### 1A. Scheduled insights MVP addendum

Status: backend implemented and tested locally. Staging scheduler, metrics, and
mobile controls are pending.

Completed:

- User-configurable weekly/monthly schedule preferences.
- Profile-timezone-aware scheduling.
- Explicit opt-in and one-step disable.
- Idempotent schedule changes and fan-out.
- Evidence threshold, ownership filters, and AI budget checks.
- Draft, deleted, rejected, and superseded content exclusion.
- `insufficient_evidence` completion.
- Structured fan-out logs.
- Integration and unit tests for enable, update, disable, duplicate prevention,
  timezone behavior, and weekly generation.

Remaining:

- Configure Cloud Scheduler or an equivalent staging trigger for
  `sela-worker -schedules`.
- Add full metrics and alerts in beta hardening.
- Add the mobile schedule controls.

Exit gate:

- A user can enable, edit, and disable a weekly or monthly schedule.
- Scheduled generation is idempotent and timezone-aware.
- A scheduled run either produces an evidence-backed insight or returns
  `insufficient_evidence`.
- Budget and ownership rules are enforced.
- No duplicate insight is produced by retries.

### 2. Staging deployment

Status: partially complete. Neon staging is provisioned and migrated to schema
`17`. Local API and worker smoke tests against Neon passed with fake providers.
Cloud Run, Cloud Scheduler, Secret Manager, and Vertex are blocked by disabled
billing and APIs. See `docs/backend/14-staging-validation.md`.

A non-GCP local staging path is available and verified end to end in
`docs/backend/15-staging-without-gcp.md`:
Docker API/worker/scheduler, Neon, OpenRouter DeepSeek, Ollama embeddings, and
ngrok. Real-provider results: 3 embeddings at 768 dimensions, 8 memory
candidates, 1 insight, Companion reply, schedule/consent lifecycle, account
purge, and a working tunnel health check.

Completed:

- Neon PostgreSQL with pgvector provisioned.
- Staging schema migrated to version `17`.
- Local API and worker smoke test against Neon:
  - auth,
  - journal -> worker,
  - Companion,
  - memory,
  - insight,
  - scheduled insight enable/disable,
  - sensitive memory consent and revocation,
  - account deletion and purge,
  - log redaction.
- Container image built.
- Staging preflight script added.

Remaining:

- Activate billing for `sela-513807`.
- Enable Cloud Run, Artifact Registry, Cloud Build, Secret Manager, Cloud
  Scheduler, and Vertex AI APIs.
- Fill `AI_API_KEY`. `VERTEX_PROJECT_ID` is set locally to `sela-513807` and
  still must be provided to the deployed environment.
- Provide reviewed crisis resources.
- Deploy API, worker, migration job, and scheduler trigger.
- Run staging smoke test with `-WorkerMode GCloud`.
- Verify backup/restore, AI budgets, and live model evaluation.

Exit gate:

- Staging URL is stable.
- Worker processes jobs reliably.
- Account deletion is verified on staging.
- Logs contain no journal or message content.

### 3. Mobile MVP — online first

Status: not started.

Order:

1. Auth and secure token storage.
2. Journal list, editor, detail, draft, search, and a small optional guided
   prompt set.
3. Companion chat.
4. Memory review, sources, correction, rejection, and deletion.
5. Insight polling, evidence, feedback, and "Bahas dengan Companion".
6. Scheduled insight opt-in, schedule editing, disable, and review.
7. Sensitive memory consent and revocation.
8. Editable profile preferences and account deletion.
9. Error, loading, and empty states.

Exit gate:

- A user can complete the core reflection loop from mobile.
- A user can review memory, review insights, and manage a scheduled insight.
- A user can grant and revoke sensitive memory consent.
- A user can edit language, timezone, display name, and communication style.
- Tokens refresh safely.
- Account deletion removes local session state.

### 4. Beta hardening

Status: local backend hardening implemented. Hosted monitoring and clinical
review remain external gates. See `docs/backend/16-beta-hardening.md`.

Completed:

- Health gates for migration state, job backlog, failed jobs, due schedules,
  AI failures, and budget usage.
- AI usage and cost report without content.
- Backup and restore verification with PostgreSQL 17 + pgvector.
- Incident runbook.
- Security and log-redaction checklist.
- Rate-limit baseline.
- Stale-memory inspection.
- Scheduled insight metrics available through job and schedule queries/logs.

Remaining:

- Hosted metrics dashboards and alert routing, deferred with GCP.
- Local clinical review of crisis resources.
- Live model quality evaluation.
- External penetration test before public beta.
- Privacy policy, data export scope, age gating, and account-deletion
  disclosure.

### 5. Offline-first journaling

Status: backend endpoint implemented. Mobile offline L1 remains deferred until
the online MVP is stable. See `docs/backend/17-offline-sync.md`.

Completed:

- `GET /journal-changes`.
- `updated_at, id` cursor pagination.
- Tombstones for deleted journals.
- Draft inclusion.
- Ownership isolation.
- Conflict rules documented and covered by existing `If-Match` behavior.
- Integration coverage for pagination, ordering, tombstones, drafts, and
  cross-user isolation.

Remaining mobile work:

- Encrypted local journal database.
- Outbox and retry.
- Sync on foreground/resume.
- Conflict detection and resolution UI.
- Cleanup on logout and account deletion.

## Required inputs

### Immediate

| Input | Needed for | Free option | Notes |
|---|---|---|---|
| Vertex embedding credentials | Real retrieval | No | Fixed decision: `gemini-embedding-001` at 768 dimensions. Requires a GCP project, region, billing, and ADC/service account. |
| Neon project or PostgreSQL URL | Staging database | Neon free tier | Need pooled and direct URLs for pgvector. |
| Hosting project and billing | API + worker | Limited free tiers | Worker must be long-running, not request-only. |
| Production LLM key and budget | Real staging responses | No | OpenRouter or OpenCode paid inference; not the Go coding plan. |
| Named safety reviewer | Crisis resources | No | Indonesian resources and escalation copy must be reviewed. |

### Mobile and beta

- Mobile repository and Flutter project.
- App identifier and signing keys.
- Google Play developer account for closed testing.
- Privacy policy URL and support email.
- Account deletion disclosure URL.
- Reviewed copy for sensitive memory consent and revocation.
- Reviewed copy for scheduled insight opt-in, cadence, and disable behavior.

### Operations

- Observability provider choice and privacy-redaction policy.
- Backup destination and retention window.
- Per-user and global monthly AI budget limits.

## Secret handling

Do not paste secrets into chat or commit them. Store credentials in `.env`
locally and in the hosting provider's secret manager for staging. `.env` is
ignored by Git.
