# SELA Product Vision Decisions

Version: 1.1
Status: Accepted
Date: 2026-11-21

## 1. Purpose

This document records the product decisions made while reconciling the SELA
product vision with the canonical product and technical documentation.

It is the tie-breaker when documents disagree. It does not replace the detailed
product specification, frameworks, or roadmap:

- Product detail: `01-product-specification.md`
- Reflection frameworks: `02-product-frameworks.md`
- Delivery sequence: `09-roadmap.md`
- Pre-beta execution: `docs/backend/11-focus-phase.md`

A decision in this document changes scope and documentation. It does not assert
that the backend, mobile application, OpenAPI contract, or database schema
already implement it.

## 2. Decision Summary

| # | Area | Decision | Delivery |
|---|---|---|---|
| 1 | Scheduled insights | Weekly and monthly reflections are MVP. User opt-in, timezone-aware, and user-configurable. | MVP |
| 2 | Journal analysis | A core product capability, but post-MVP delivery. | Post-MVP |
| 3 | Frameworks and Life Priority | Internal, selective guidance. Not exposed as labels or scores. | MVP prompt guidance |
| 4 | Companion challenge and adaptive style | Strengthen the Companion prompt now. Define healthy challenge and adaptive behavior explicitly. | Prompt v2 active; live quality evaluation pending |
| 5 | "Explore further" | Handled through Companion, not as a new insight feedback action. | MVP UX |
| 6 | Sensitive memory | Store only with explicit user consent. Default remains skip. | MVP backend implemented; mobile UI pending |
| 7 | Fixed technical decisions | Keep HS256 + rotating refresh, asynchronous `202` + job polling, and Vertex `gemini-embedding-001` at 768 dimensions. | Fixed |
| 8 | Memory vocabulary | Keep internal `candidate`, `active`, `rejected`, `superseded`, `deleted`. User-facing copy uses "digantikan" and "dihapus"; do not use "archived" without a real archive feature. | Fixed |

## 3. Decision Details

### 3.1 Scheduled insights are MVP

Weekly and monthly reflections are part of the MVP.

Requirements:

- Scheduled insights are opt-in.
- Users can choose weekly or monthly cadence and disable the schedule at any
  time.
- Scheduling is timezone-aware and uses the user's profile timezone.
- The same evidence threshold as on-demand insights applies:
  at least two distinct eligible sources and confidence of at least 0.40.
- If evidence is insufficient, no insight is created and the user receives an
  explicit `insufficient_evidence` outcome.
- Scheduled generation must be idempotent, cost-controlled, and subject to the
  existing AI budgets and rate limits.
- Drafts, deleted content, rejected memories, superseded memories, and content
  outside the user's ownership are excluded.
- In-app delivery is required. Push notification delivery is a separate
  decision.
- Users must be able to see why a scheduled insight was generated and which
  period and sources it used.

Implementation status: backend implemented. The schedule API, timezone-aware
fan-out, idempotent job creation, evidence threshold, and budget checks are in
place. The staging scheduler trigger and mobile controls remain pending. See
`docs/backend/11-focus-phase.md`.

### 3.2 Journal analysis is core but post-MVP

Per-entry journal analysis remains a core product capability. Delivery is
post-MVP.

This resolves the two instructions given in the same review:

- "jadikan fitur inti" means the capability is core to the product.
- "post mvp" means delivery is after the MVP.

MVP journal value is delivered through free writing, Companion reflection, and
period-based insights.

When per-entry analysis is delivered:

- It must be tied to a specific journal entry.
- It must use the same evidence and uncertainty standards as insights.
- The analysis must be stored separately from the original journal.
- The original journal must remain unchanged.
- The user must be able to request, inspect, and delete the analysis.

Implementation status: not implemented. No per-entry analysis endpoint or mobile
action is part of the MVP.

### 3.3 Frameworks and Life Priority are internal guidance

Frameworks are thinking tools, not user-facing labels.

Rules:

- Select frameworks by relevance.
- Do not force a framework into every response.
- Do not name a framework unless the user asks or naming it clearly helps.
- Use at most one or two frameworks in a single response.
- The Life Priority Framework orders reflection and next steps. It is not a
  score, a diagnosis, or a mandatory sequence.
- `Improve`, `Maintain`, and `Accept` remain the only user-facing reflection
  directions in the MVP.
- Framework use must be evaluated for relevance, non-diagnosis, tone, and user
  burden.

Implementation status: framework selection guidance is active in prompt v2 and
covered by deterministic behavior tests. No new user-facing priority field or
score is added to the MVP schema.

### 3.4 Companion challenge and adaptive style are strengthened now

The Companion must be empathetic but not blindly agreeable.

Required behavior:

- It may challenge an unsupported assumption when the available context
  supports the challenge.
- It must acknowledge the user's experience before offering an alternative
  perspective.
- It must not shame, moralize, or force a conclusion.
- Adaptive style is behavioral:
  - gentle when the user is distressed;
  - structured when the user is confused;
  - direct when avoidance is clear;
  - practical when the user asks for action;
  - non-advisory when the user only wants to be heard.
- It must ask a focused question when a question helps the user think.
- It must not invent context or present an interpretation as fact.

Implementation status: `prompts/companion/v2.md` is the active prompt.
Deterministic behavior and prompt-injection gates pass. Live model quality
evaluation remains a separate manual step.

### 3.5 "Explore further" goes through Companion

Insight feedback remains:

- `confirm`
- `correct`
- `reject`

The API does not add a fourth feedback action in the MVP.

The client may offer "Bahas dengan Companion" or an equivalent action that
starts a conversation with the insight context. This keeps the feedback model
simple and gives the user a real reflective path instead of a dead-end action.

Implementation status: no API change. The mobile action is an MVP UX item.

### 3.6 Sensitive memory requires explicit consent

Default behavior remains: sensitive content is not extracted into long-term
memory.

With explicit user consent:

- Store only the minimum necessary sensitive memory.
- Retain provenance and confidence.
- Restrict retrieval to operations covered by the consent.
- Allow the user to inspect, correct, reject, and delete the memory.
- Allow the user to revoke consent at any time.
- Do not use sensitive memories for model training.
- Re-evaluate or delete stale sensitive memories.

Consent must be specific, recorded, and revocable. It must not be bundled into a
generic "accept all" action.

Implementation status: backend implemented. Consent storage, grant/revoke API,
sensitive extraction with consent, sensitivity metadata, and retrieval
exclusion are in place. Mobile consent UI remains pending.

### 3.7 Fixed technical decisions

The following decisions were listed as open in the product vision. They are
fixed for the MVP by the existing implementation and architecture:

- Authentication: HS256 access tokens with rotating refresh tokens, session
  revocation, and refresh-token reuse detection.
- Asynchronous insight generation: `202 Accepted` with a job identifier and a
  status operation.
- Embeddings: Vertex AI `gemini-embedding-001` at 768 dimensions.

Reopening any of these requires a new decision record and an evaluation of the
impact on the contract, schema, mobile client, and operations.

### 3.8 Memory vocabulary

Internal statuses remain:

- `candidate`
- `active`
- `rejected`
- `superseded`
- `deleted`

User-facing copy:

- `candidate`: "kandidat"
- `active`: "aktif"
- `rejected`: "ditolak"
- `superseded`: "digantikan"
- `deleted`: "dihapus"

Do not use "archived" unless an archive feature is explicitly added. If an
archive feature is added later, it needs its own status, retention rule, and
decision record.

## 4. Consequences

The following documents are updated to reflect these decisions:

- `01-product-specification.md`: scheduled insights moved to MVP; journal
  analysis marked core/post-MVP; sensitive memory consent added; metrics
  expanded.
- `02-product-frameworks.md`: framework operationalization and Companion
  behavior added.
- `03-system-architecture.md`: scheduled insights are MVP; sensitive memory
  consent gate added.
- `04-database-design.md`: scheduled insight preferences are no longer deferred;
  memory vocabulary and sensitive memory policy clarified.
- `05-ai-system-design.md`: scheduled insights are MVP; framework guidance,
  Companion behavior, and sensitive memory consent added.
- `06-api-contract.md`: no fourth insight feedback action; scheduled insight
  preferences are MVP and require contract work.
- `07-security-and-privacy.md`: sensitive memory consent and retention policy.
- `08-operations-and-reliability.md`: scheduled insights are MVP; scheduling,
  idempotency, timezone, budget, and observability requirements.
- `09-roadmap.md`: scheduled insights moved to Phase 2 Core MVP; journal
  analysis remains post-MVP; current decisions updated.
- `docs/backend/02-backend-implementation.md`: insight phase now includes
  scheduled insight preferences and sensitive memory consent.
- `docs/backend/03-ai-integration.md`: scheduled insights are MVP; framework
  guidance, Companion behavior, prompt v2, and sensitive memory consent.
- `docs/backend/04-testing-and-deployment.md`: acceptance tests for scheduled
  insights, sensitive memory consent, and prompt v2 evaluation.
- `docs/backend/10-phase-2-freeze.md`: post-freeze scope addendum.
- `docs/backend/11-focus-phase.md`: scheduled insights, prompt v2, sensitive
  memory consent, and mobile memory/insight UI added to pre-beta work.
- `docs/backend/12-cloud-run-neon-vertex-setup.md`: worker setup notes that the
  staging scheduler trigger is still pending.
- `docs/backend/13-staging-runbook.md`: staging pass notes which MVP addendum
  items are not yet verifiable.
- `README.md`, `HANDOFF.md`, and `CHANGELOG.md`: status, handoff, and change
  notes.

## 5. B1 Backend Delivery

Implemented in the B1 backend addendum at migration `17` and OpenAPI `1.4.0`:

- `insight_schedules` schema and schedule preference API.
- Weekly and monthly timezone-aware fan-out with idempotent job creation.
- `sela-worker -schedules` scheduler entry point.
- Budget-blocked scheduled generation outcome.
- `memory_consents` schema and consent API.
- Sensitive memory extraction only with active consent.
- Sensitivity metadata on memories.
- Sensitive memory exclusion from Companion and Insight retrieval without
  consent.
- Active Companion and Insight prompt v2.
- Unit, integration, contract, and deterministic behavior tests.

## 6. Open Implementation Items

The following remain open:

- Cloud Scheduler or equivalent staging trigger for `sela-worker -schedules`.
- Notification and reminder delivery.
- Mobile memory review and insight review screens.
- Mobile scheduled insight controls and consent UI.
- Editable communication preferences in mobile.
- Guided-writing prompt library scope and copy.
- Session inventory and revoke-all.
- Data export, age gating, and consent center.
- Product analytics instrumentation for activation, continuity, usefulness, and
  trust metrics.

## 7. Contradiction Resolution

Two instructions in the review could be read as contradictory:

1. Journal analysis: "jadikan fitur inti".
2. Journal analysis: "post mvp".

Resolution: journal analysis is a core product capability, but its delivery is
post-MVP. This preserves the product intent without expanding MVP scope.

If a later decision changes the delivery timing, update this document first and
then update the affected roadmap, contract, and implementation documents.
