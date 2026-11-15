# Focus Phase — Pre-Beta

- Starts after: backend `v0.2.0`
- Goal: move from a frozen local backend to a real-provider staging system and
  an online-first mobile MVP.
- This phase does not start until the required inputs below are available.

## Ordered workstreams

### 1. Real embeddings and live evaluation

Status: not started.

Tasks:

- Choose Ollama (`nomic-embed-text`) or Vertex AI.
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
  - latency and cost.

Exit gate:

- Retrieval works with real vectors.
- Evaluation report is documented.
- No unapproved provider receives private content.

### 2. Staging deployment

Status: not started.

Tasks:

- Provision PostgreSQL with pgvector.
- Deploy API and a long-running worker.
- Run migrations.
- Configure secrets and provider credentials.
- Configure domain or temporary host URL.
- Run staging smoke tests:
  - auth,
  - journal -> worker,
  - Companion,
  - memory,
  - insight,
  - account deletion.

Exit gate:

- Staging URL is stable.
- Worker processes jobs reliably.
- Account deletion is verified on staging.
- Logs contain no journal or message content.

### 3. Mobile MVP — online first

Status: not started.

Order:

1. Auth and secure token storage.
2. Journal list, editor, detail, draft, search.
3. Companion chat.
4. Memory review.
5. Insight polling, evidence, feedback.
6. Profile preferences and account deletion.
7. Error, loading, and empty states.

Exit gate:

- A user can complete the core reflection loop from mobile.
- Tokens refresh safely.
- Account deletion removes local session state.

### 4. Beta hardening

Status: not started.

Tasks:

- Request, error, latency, database, AI, and job metrics.
- Alerts for critical failures, job backlog, and budget thresholds.
- Cost dashboard.
- Backup and restore verification.
- Incident runbook.
- Local clinical review of crisis resources.
- Privacy policy and account-deletion disclosure.

### 5. Offline-first journaling

Status: deferred until the online MVP is stable.

Tasks:

- Implement `GET /journal-changes`.
- Encrypted local journal database.
- Outbox and retry.
- Conflict detection and resolution UI.
- Cleanup on logout and account deletion.

## Required inputs

### Immediate

| Input | Needed for | Free option | Notes |
|---|---|---|---|
| Embedding choice and credentials | Real retrieval | Ollama locally | Vertex requires a GCP project, region, billing, and ADC/service account. |
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

### Operations

- Observability provider choice and privacy-redaction policy.
- Backup destination and retention window.
- Per-user and global monthly AI budget limits.

## Secret handling

Do not paste secrets into chat or commit them. Store credentials in `.env`
locally and in the hosting provider's secret manager for staging. `.env` is
ignored by Git.
