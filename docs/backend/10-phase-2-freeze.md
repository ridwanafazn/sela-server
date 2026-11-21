# Phase 2 Backend Freeze

> Historical snapshot. This document intentionally records the `v0.2.0`
> freeze at schema `16` and OpenAPI `1.3.0`. Current versions are in
> `README.md`, `HANDOFF.md`, and `docs/README.md`.

- Release: `v0.2.0`
- API contract: OpenAPI `1.3.0`
- Database schema: migration version `16`
- Date: 2026-11-15
- Scope: backend only

## Status

Phase 2 backend is complete and frozen. The mobile application and staging
deployment are separate deliverables and are not included in this release.

## Post-freeze scope addendum — 2026-11-21

A product vision reconciliation changed the MVP scope after this freeze:

- Weekly and monthly scheduled insights are promoted to MVP.
- Per-entry journal analysis remains a core capability with post-MVP delivery.
- Sensitive memory storage requires explicit, revocable user consent; the
  default remains skip.
- Companion and insight prompt v2 are active.

The B1 backend addendum is implemented at migration `17` and OpenAPI `1.4.0`:
schedule API, timezone-aware fan-out, consent API, sensitive retrieval
enforcement, and active prompt v2. The staging scheduler trigger and mobile
controls remain pending.

The `v0.2.0` release remains a historical record. The addendum is not part of
the frozen artifact. Execution is tracked in
`docs/backend/11-focus-phase.md` and the decisions are recorded in
`docs/shared/10-product-vision-decisions.md`.

## Exit criteria

| Criterion | Status |
|---|---|
| A journal produces memory candidates | Complete |
| Companion uses authorized personal context | Complete |
| Insights include evidence and confidence | Complete |
| Insufficient evidence returns an explicit outcome | Complete |
| Cross-user authorization is tested | Complete |
| Account deletion is tested end to end | Complete |
| AI calls have timeouts, retries, and usage tracking | Complete |
| Journal, Companion, memory, and insight APIs match OpenAPI | Complete |

## Verification performed

- Unit tests.
- Full integration suite against disposable PostgreSQL + pgvector databases.
- OpenAPI response contract validation.
- Deterministic safety and prompt-injection evaluation gates.
- Vulnerability scan.
- Container build.
- Real-provider E2E with OpenCode Go:
  - journal creation,
  - embedding and memory extraction jobs,
  - Companion response with journal and memory context,
  - asynchronous insight with evidence,
  - AI usage events,
  - account deletion and purge.

## Deferred to Phase 3

- Encrypted local mobile journal storage.
- Mobile outbox, retry, and conflict-resolution UI.
- Automatic background sync.
- Metrics dashboards, alerts, backup/restore verification, and incident runbook.
- Live model quality evaluation for tone, factual accuracy, and hallucination
  rate.
- Locale clinical review of crisis resources.
- Staging and production deployment.

Per-entry journal analysis is a core capability with post-MVP delivery and is
not part of Phase 3. Scheduled insights are an MVP addendum, not deferred.

## Required inputs for the next focus

The ordered pre-beta workstreams and required inputs are tracked in
`docs/backend/11-focus-phase.md`.

### Immediate

1. Embedding decision:
   - local Ollama with `nomic-embed-text`, or
   - Vertex AI project, region, and service account / ADC using
     `gemini-embedding-001` at 768 dimensions.
2. Staging database:
   - Neon project and pooled + direct connection strings, or an equivalent
     PostgreSQL with pgvector.
3. Staging host:
   - Cloud Run/GCP project and billing account, or another host for the API and
     a long-running worker.
4. Production LLM:
   - OpenRouter API key and monthly budget, or OpenCode paid inference service
     account.
5. Safety review:
   - a named reviewer for Indonesian crisis resources and escalation copy.

### For mobile and beta

- Mobile repository and Flutter project.
- App identifier and signing keys.
- Google Play developer account for closed testing.
- Privacy policy URL and support email.
- Account deletion disclosure URL.

### For Phase 3 operations

- Observability provider choice and privacy-redaction policy.
- Backup destination and retention window.
- Monthly user/global AI budget limits.
