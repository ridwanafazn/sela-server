# SELA Roadmap

Version: 1.2  
Status: Phase 0 complete; Phase 1 ready to start

## 1. Purpose

This document defines the product and engineering phases for SELA.

It prevents scope creep by making the boundary between the current MVP, beta hardening, and post-MVP work explicit.

P0, P1, and P2 in earlier review notes were documentation priority levels. They are not product phases. This roadmap uses Phase 0 through Phase 4.

## 2. Phase Summary

| Phase | Goal | Primary outcome |
|---|---|---|
| Phase 0 | Contract lock | Stable API, database, and decision baseline |
| Phase 1 | Walking skeleton | End-to-end authentication and journal flow on staging |
| Phase 2 | Core MVP | Journal, Companion, memory, and insight working end to end |
| Phase 3 | Beta hardening | Secure, observable, offline-capable journaling ready for beta |
| Phase 4 | Post-MVP depth | Deeper reflection features, privacy controls, and scale work |

## 3. Phase 0 — Contract Lock

Status: complete.

Deliverables:

- Canonical OpenAPI 3.1 contract.
- Conceptual DBML.
- P0 decisions for routes, insight lifecycle, memory status, journal drafts, auth, deletion, errors, and idempotency.
- Validation report.

Exit criteria:

- OpenAPI validates.
- DBML converts to PostgreSQL.
- Cross-document conflicts are resolved or explicitly deferred.
- No implementation starts from conflicting route maps.

## 4. Phase 1 — Walking Skeleton

Goal: prove the full path from mobile to database with a small, real feature set.

Backend:

- Go application bootstrap.
- Configuration and structured logging.
- Neon PostgreSQL connection.
- SQL migration tool and initial migrations.
- Health endpoints.
- Registration, login, refresh, logout.
- Current user and profile.
- Journal CRUD with server-side drafts.
- Journal version field and optimistic concurrency foundation.
- Docker image and Cloud Run staging deployment.
- GitHub Actions checks.

Mobile:

- Flutter shell, theme, and routing.
- Authentication and secure token storage.
- API client, error mapping, and token refresh.
- Journal list, editor, detail, search, and draft autosave.
- Loading, empty, error, and offline states.

Infrastructure:

- Development and staging environments.
- Database migration process.
- Artifact Registry image push.
- Basic staging smoke test.

Exit criteria:

- A user can register, log in, create a draft, publish it, edit it, list it, and delete it on staging.
- Session refresh works.
- Ownership checks are tested.
- No journal content appears in logs.
- CI passes.

Out of scope:

- AI generation.
- Memory extraction.
- Insights.
- Offline local database.
- Scheduled insights.

## 5. Phase 2 — Core MVP

Backend status: complete and frozen at `v0.2.0` (2026-11-15). Mobile delivery
and staging deployment remain separate work.

Goal: deliver the core reflection loop.

Backend:

- Companion conversations and messages.
- Idempotent message sending.
- Vertex AI LLM adapter.
- Embedding generation for active journals.
- Hybrid retrieval with PostgreSQL full-text search and pgvector.
- Memory extraction jobs and candidate review.
- Insight generation jobs with evidence.
- Insight feedback.
- Account deletion job.
- Basic communication preferences.

Mobile:

- Companion conversation list and chat.
- Memory list, detail, sources, and feedback.
- Insight list, asynchronous generation, job polling, evidence detail, and feedback.
- Profile, communication preferences, and account deletion.
- Privacy explanation and deletion consequences.

AI:

- Prompt templates and versioning.
- Evidence threshold enforcement.
- Safety escalation.
- Evaluation dataset and quality gates.
- Token and cost controls.

Exit criteria:

- A journal leads to memory candidates.
- Companion uses authorized personal context.
- An insight includes evidence and confidence.
- Insufficient evidence returns an explicit outcome, not a fabricated insight.
- Cross-user authorization tests pass.
- Account deletion is tested.
- AI operations have timeouts, retries, and usage tracking.

Out of scope:

- Journal analysis.
- Scheduled insights.
- Privacy center.
- Full offline-first.
- Multi-device sync.

## 6. Phase 3 — Beta Hardening

Goal: make the MVP safe, observable, and usable in real conditions.

Security and reliability:

- Security review.
- Rate-limit implementation.
- Deletion verification.
- Backup and restore test.
- Sensitive logging review.
- Dependency and container scanning.
- Incident runbook.

Observability:

- Request, error, latency, database, AI, and job metrics.
- Alerts for critical failures.
- Cost dashboards.

Mobile quality:

- Accessibility pass.
- Localization pass.
- Performance pass.
- Crash reporting with privacy redaction.
- Play Store internal and closed testing.
- Privacy policy and data deletion disclosure.

Offline journaling:

- Encrypted local journal database.
- Journal outbox.
- Sync on foreground and resume.
- Conflict detection and resolution UI.
- Offline and sync status indicators.
- Local database cleanup on logout and account deletion.

Exit criteria:

- Beta release is available to closed testers.
- Offline journal create, edit, and delete sync correctly.
- Conflicts do not silently overwrite user content.
- No critical security issue remains.
- Account deletion is verified end to end.
- Crash-free session rate meets the beta target.
- Monitoring and alerts are active.

## 7. Phase 4 — Post-MVP Depth

Goal: add depth only when product evidence supports it.

Candidates:

- Journal analysis.
- Scheduled insights.
- Privacy center and granular consent.
- Data export.
- Push notifications.
- Encrypted read cache for journals.
- Full offline-first for journals.
- Multi-device sync and conflict resolution.
- Advanced search.
- Memory graph and improved retrieval.
- Additional localization.
- Cost and model optimization.

Rules:

- Each candidate needs a product metric or user-research reason.
- Do not add Redis, microservices, or a separate inference service without evidence.
- Do not enable multi-device sync before conflict and deletion behavior is tested.
- Do not cache sensitive AI content locally without encryption and a retention policy.

## 8. Offline Strategy

| Level | Capability | Status |
|---|---|---|
| L0 | Online-first, no local content | Superseded |
| L1 | Encrypted journal outbox and sync | Chosen for Phase 3 |
| L2 | Encrypted read cache for journals | Phase 4 candidate |
| L3 | Full offline-first with multi-device sync | Phase 4 candidate, evidence required |

L1 scope:

- Journal content and drafts only.
- SQLite via Drift.
- SQLCipher encryption.
- Key stored in Android Keystore or iOS Keychain.
- Outbox for create, update, and delete.
- Sync on app foreground and resume.
- No background sync in the first implementation.
- Companion and Insights remain online-only.

AI behavior offline:

- Companion and insight generation are unavailable.
- The UI explains that AI features require a connection.
- Active journals are queued for embedding and memory extraction after sync.

## 9. Phase Gates

A phase starts only when:

- The previous phase exit criteria are met.
- The relevant contract changes are merged.
- Security and privacy implications are reviewed.
- Required infrastructure is available.
- The team agrees on what is explicitly out of scope.

A phase does not expand silently. New work goes into the next phase unless it is required to fix a production or security issue.

## 10. Current Decisions

- OpenAPI is the canonical API contract.
- Insight generation is asynchronous.
- Journal drafts are server-side, with encrypted local outbox in Phase 3.
- Memory statuses are candidate, active, rejected, superseded, and deleted.
- Embeddings use Vertex AI with 768 dimensions.
- Authentication uses HS256 with rotating refresh tokens.
- Account deletion is in the MVP.
- Journal analysis and scheduled insights are post-MVP.
- Privacy center and data export are post-MVP.
