# Session Handoff

Last updated: 2026-11-21 (B1 + local beta hardening)

## Current state

- Repository: `C:\Users\Ridwan\Git-v2\sela-server`
- Branch: `main`, synced with `origin/main`
- Latest commit:
  - `3bdee33` — complete backend addendum and local beta hardening
- Working tree: clean.
- Local staging containers: stopped.
- Release tags:
  - `v0.2.0` — Phase 2 backend freeze
  - `v0.2.1` — Vertex 768-dimensional embedding compatibility
- API contract: OpenAPI `1.4.0`
- Database schema: migration version `17` (pre-beta addendum; `v0.2.0` shipped at 16)
- Backend Phase 2: complete and frozen (`10/10` internal stages)

## Product vision reconciliation — 2026-11-21

The product vision review changed the MVP scope. Decisions are recorded in
`docs/shared/10-product-vision-decisions.md`.

- Weekly and monthly scheduled insights are promoted to MVP.
- Per-entry journal analysis is a core capability with post-MVP delivery.
- Frameworks and Life Priority remain internal guidance, not user-facing labels
  or scores.
- Companion and insight prompt v2 are active. Deterministic safety,
  prompt-injection, and behavior gates pass; live model quality evaluation
  remains manual.
- Insight feedback remains confirm/correct/reject. "Explore further" goes
  through Companion.
- Sensitive memory requires explicit, revocable consent; the default remains
  skip.
- Fixed technical decisions: HS256 + rotating refresh, asynchronous `202` + job
  polling, Vertex `gemini-embedding-001` at 768 dimensions.
- Memory vocabulary: internal `superseded`/`deleted`; user-facing
  "digantikan"/"dihapus"; no "archived" without an archive feature.

The canonical docs, README, roadmap, security/privacy, operations, backend AI
integration, testing, freeze addendum, and focus phase were updated. The B1
implementation added code, schema, OpenAPI, and active prompt v2.

## B1 backend addendum — 2026-11-21

Implemented:

- Migration version `17`: `insight_schedules`, `memory_consents`, and
  `personal_memories.sensitivity`.
- `GET`, `PUT`, and `DELETE /insight-schedule`.
- Weekly/monthly fan-out with `sela-worker -schedules`.
- `GET` and `PUT /memory-consent`.
- Sensitive memory extraction only with active consent.
- Sensitive memory exclusion from Companion and Insight retrieval without
  consent.
- Active `companion/v2` and `insight/v2`.
- OpenAPI `1.4.0` contract updates.

Verification:

- `go test ./...` passed.
- `go test -tags=integration ./...` passed against PostgreSQL + pgvector.
- OpenAPI lint is valid with four pre-existing warnings.
- Unit and integration tests cover schedule lifecycle, fan-out idempotency,
  weekly generation, consent grant/revoke, sensitive extraction, and retrieval
  exclusion.

## B2 staging validation — 2026-11-21

Completed:

- Neon staging migrated from schema `16` to `17`.
- Verified `insight_schedules`, `memory_consents`, and
  `personal_memories.sensitivity`.
- Local API and worker ran against Neon staging with fake providers.
- Smoke test passed: health, auth, schedule lifecycle, consent lifecycle,
  journals, worker, Companion, insight request, account deletion, purge, and
  deleted-account login block.
- No journal or message content appeared in API logs.
- Container image `sela-server:b2-local` built.
- Added `scripts/staging-preflight.ps1`.
- Added `docs/backend/14-staging-validation.md`.
- Added and verified a non-GCP local staging path:
  - `compose.staging.yml`,
  - `scripts/staging-local-init.ps1`,
  - `scripts/staging-local.ps1`,
  - `docs/backend/15-staging-without-gcp.md`.
- Real-provider verification:
  - OpenRouter `deepseek/deepseek-v4.1-flash-1017`,
  - Ollama `nomic-embed-text` 768 dimensions,
  - 3 embeddings, 8 memory candidates, 1 insight,
  - Companion reply, schedule/consent lifecycle, account purge,
  - ngrok tunnel health check.

GCP status:

- Billing is disabled for `sela-513807`.
- Cloud Run, Artifact Registry, Cloud Build, Secret Manager, Cloud Scheduler,
  and Vertex AI APIs are disabled.
- OpenRouter `AI_API_KEY` is configured locally.
- `VERTEX_PROJECT_ID` is set to `sela-513807` locally.
- `SAFETY_RESOURCES_FILE` is still empty; crisis resources remain unreviewed.
- GCP is intentionally deferred in favor of the verified local staging path.

## Phase 3 offline backend — 2026-11-21

Implemented:

- `GET /journal-changes`.
- `updated_at, id` cursor pagination.
- Tombstones with `journal = null`.
- Draft inclusion.
- `sync.server_time`.
- Ownership isolation.
- Integration coverage for pagination, ordering, tombstones, drafts, and
  cross-user isolation.

Mobile offline L1 remains pending: encrypted local database, outbox, foreground
sync, conflict-resolution UI, and local cleanup on logout/deletion.

## Pre-Beta position

Workstreams from `docs/backend/11-focus-phase.md`:

1. Real Vertex embeddings and live model evaluation — not started. Prompt v2 is
   active; deterministic gates pass.
2. Scheduled insights MVP addendum — backend implemented; staging scheduler
   trigger pending.
3. Staging deployment — Neon schema `17` is applied and locally smoke-tested;
   GCP deployment is blocked on billing and disabled APIs.
4. Mobile MVP, including memory, insights, schedule controls, and editable
   profile preferences — not started.
5. Beta hardening - local backend checks implemented; hosted monitoring and
   clinical review pending. See `docs/backend/16-beta-hardening.md`.
6. Offline-first - backend `GET /journal-changes` implemented; mobile L1
   deferred.

The local staging path is verified and is the active path. The GCP runbook
(`docs/backend/13-staging-runbook.md`) remains available if billing is enabled
later. The next backend focus is mobile integration and the remaining external
gates: crisis-resource review, live model quality review, and public-beta
privacy/legal work.

## Mobile Phase 1 (2026-11-16)

- Repository: `C:\Users\Ridwan\Git-v2\sela-mobile` (local commit `6295147`).
- Visual direction: Dawn Paper — warm gradients, soft orbs, floating pill
  navigation, Plus Jakarta Sans + Lora.
- Phase 1 walking skeleton code is complete:
  GoRouter, auth/session, journal CRUD, search and filters, server-side draft
  autosave, unsaved-changes guard, onboarding, Companion chat, profile, and
  re-authenticated account deletion.
- Verification: `flutter analyze` clean, `flutter test` 12 passing,
  `flutter build web` passing, authenticated flow captured at 390x844.
- Pending: staging deployment (billing), Android emulator/device validation,
  Insights and memory (Phase 2), offline L1 (Phase 3).
- Authoritative mobile docs:
  `C:\Users\Ridwan\Documents\Default Project\sela-documentation\sela-mobile\docs\flutter`.
- The docs package ships OpenAPI 1.2.0; the backend repository is 1.4.0.
  Use the repository contract until the documentation is reconciled.

## Environment status

All project dev services are stopped at handoff:

- Local staging containers `sela-staging-local-*` stopped.
- Ollama installed with `nomic-embed-text` (274 MB); model unloaded.
- ngrok installed; no tunnel running.
- `sela-api-smoke` and `sela-mock-llm` containers removed.
- `sela-server-postgres-1` stopped and removed via `docker compose down`.
- PostgreSQL data volume retained:
  `sela-server_postgres-data`

To resume local staging:

```powershell
Set-Location "C:\Users\Ridwan\Git-v2\sela-server"
.\scripts\staging-local.ps1 up
```

Stop it when finished:

```powershell
.\scripts\staging-local.ps1 down
```

Local dev without Docker staging:

```powershell
docker compose up -d
go run ./cmd/migrate up

# Terminal 1
go run ./cmd/api

# Terminal 2
go run ./cmd/worker
```

Optional Swagger UI:

```powershell
docker run -d --name sela-swagger -p 8090:8080 `
  -e SWAGGER_JSON=/openapi.yaml `
  -v "${PWD}/docs/shared/openapi.yaml:/openapi.yaml:ro" `
  swaggerapi/swagger-ui
```

Open `http://localhost:8090`.

## Local verification — 2026-11-16

Completed against the frozen Phase 2 code:

- `go test ./...` passed.
- `go test -count=1 -tags=integration ./...` passed (27.7s).
- `gofmt -l .` clean; `go vet ./...` clean.
- Migrations `up` -> `down -all` -> `up` on a disposable database: version 16.
- Container image `sela-server:v0.2.1-local` built.
- Container smoke test `scripts/local-smoke.ps1`:
  health, register, journals, worker, memories, conversation, companion,
  insight accept, account delete, purge, login blocked.
- Mock OpenAI-compatible container test (`scripts/mock-llm.py`, the same
  adapter path used for OpenRouter): memory candidate created, insight
  completed with three evidence sources, account purge completed.
- API logs contain no journal or message content.
- Neon schema dump (53 KB) restored locally: version 16, 18 tables,
  pgvector 0.8.7.

With the built-in fake provider, the insight job fails because it returns a
fixed memory JSON payload. The mock provider and the integration suite cover the
real insight path.

Run the mock provider on the compose network when needed:

```powershell
docker run -d --name sela-mock-llm --network sela-server_default `
  -v "${PWD}/scripts:/app:ro" python:3.13-alpine python /app/mock-llm.py
```

Then start the API with `AI_PROVIDER=openai`,
`AI_BASE_URL=http://sela-mock-llm:9090/v1`, `AI_API_KEY=test`, and
`AI_MODEL=mock-model`.

## Local configuration

`.env` is local, ignored by Git, and contains the current development
credentials. Do not commit it.

Current local state:

- LLM: OpenCode Go OpenAI-compatible endpoint.
- Model: `deepseek-v4.1-flash`.
- Embeddings: Ollama `nomic-embed-text` (768 dimensions) for local staging.
- Database: Neon PostgreSQL with pgvector.
- Safety resources: embedded defaults, flagged as unreviewed.
- `.env.staging` (gitignored): Neon pooled/direct URLs, JWT and rate-limit
  secrets, and OpenRouter `AI_API_KEY`. `VERTEX_PROJECT_ID` is set to
  `sela-513807`.
- `.env.staging.local` (gitignored): local staging overrides.
- gcloud account is authenticated and the active project is `sela-513807`.

Before staging or real journal use:

- use only OpenRouter `deepseek/deepseek-v4.1-flash-1017`,
- keep embeddings on Ollama `nomic-embed-text` for local staging,
- configure reviewed crisis resources,
- configure AI pricing and budgets.

## Selected next-phase architecture

Active path:

- API/worker/scheduler: local Docker
- Database: Neon PostgreSQL with pgvector
- Embeddings: Ollama `nomic-embed-text` at 768 dimensions
- Text generation: OpenRouter `deepseek/deepseek-v4.1-flash-1017`
- Public URL: ngrok
- Target device: Samsung Galaxy A35 (Android)

Deferred path:

- API: Google Cloud Run
- Embeddings: Vertex AI `gemini-embedding-001`
- Setup guide: `docs/backend/12-cloud-run-neon-vertex-setup.md`

Local staging guide:

`docs/backend/15-staging-without-gcp.md`

Local runtime:

- `sela-api`
- `sela-worker`
- `sela-scheduler`
- host Ollama
- ngrok tunnel

## What the next session needs

### Accounts and access

1. Neon project in `aws-ap-southeast-1` (Singapore) with pooled and direct URLs.
2. OpenRouter account, paid credits, key spend limit, and ZDR/no-training
   privacy settings.
3. Ollama for Windows with `nomic-embed-text`.
4. ngrok account/configuration for the HTTPS tunnel.
5. A named reviewer for Indonesian crisis resources.
6. Mobile repository / Flutter project for the Android app.
7. Google Cloud billing only if the deferred GCP path is revived.

### Credentials

Do not send secrets in chat or commit them. Put values in:

- `.env.staging` for staging,
- `.env.staging.local` for local staging,
- Google Secret Manager only if the GCP path is revived.

Required local staging values:

- pooled Neon URL,
- direct Neon URL for migrations,
- JWT access secret,
- rate-limit secret,
- OpenRouter API key.

### Local tooling status

- Docker: installed and running.
- Flutter: `3.47.0` installed; `flutter doctor` reports no issues.
- Android SDK: `36.1.0`; `adb` 1.0.41 is at
  `%LOCALAPPDATA%\Android\Sdk\platform-tools` but not on `PATH`.
- `gcloud`: `593.0.0` installed and authenticated.
- Neon MCP is authenticated to the `ridwanafazn` organization.

## Next steps

1. Start local staging:
   `.\scripts\staging-local.ps1 up`
2. Run the smoke test:
   `.\scripts\staging-local.ps1 smoke`
3. Verify health, usage, backup, and restore:
   `.\scripts\staging-local.ps1 health`
   `.\scripts\staging-local.ps1 usage`
   `.\scripts\staging-local.ps1 restore-check`
4. Review crisis resources before entering real journal data.
5. Run the live model quality evaluation with DeepSeek and Ollama.
6. Build the Android Flutter MVP against the ngrok URL, including memory review,
   insight review, schedule controls, consent UI, and editable profile
   preferences.
7. Revive the GCP path only if public beta hosting requires it.

## Known gaps

- Mobile app is in `C:\Users\Ridwan\Git-v2\sela-mobile`. Phase 1 code is
  complete, but staging and Android-device validation are pending.
- Mobile memory review, insight review, schedule controls, sensitive memory
  consent, editable profile preferences, and the guided writing prompt set are
  not implemented.
- Scheduled insights are implemented in the backend. The local scheduler runs
  in Docker; mobile controls are pending.
- Per-entry journal analysis is post-MVP and not implemented.
- Sensitive memory consent is implemented in the backend. The mobile consent UI
  is pending.
- Prompt v2 is active. Live model quality evaluation remains manual.
- Local staging is verified. GCP deployment is deferred; billing and the
  required GCP APIs are disabled.
- `.env.staging` has `AI_API_KEY`; `VERTEX_PROJECT_ID` is set locally.
- `GET /journal-changes` is implemented. Mobile encrypted storage, outbox,
  foreground sync, and conflict UI remain Phase 3 mobile work.
- Local health gates, AI usage report, backup/restore verification, and incident
  runbook are implemented. Hosted dashboards/alerts are deferred with GCP.
- Live model quality evaluation for tone, facts, and hallucination is manual.
- Embedded crisis resources need local clinical review.
- Real semantic retrieval works locally with Ollama `nomic-embed-text`.
- OpenRouter handles text generation outside GCP. A Vertex Gemini LLM adapter is
  only required if all journal text must stay inside GCP.
- Email verification and password reset are not implemented. Public users who
  forget a password cannot recover their account.
- Subscription plans, payment webhooks, entitlements, and free/premium limit
  enforcement are not implemented.
- Data export, consent/age gating, and admin/support tooling are not implemented.

## Useful docs

- `CHANGELOG.md`
- `docs/shared/10-product-vision-decisions.md`
- `docs/backend/10-phase-2-freeze.md`
- `docs/backend/11-focus-phase.md`
- `docs/backend/12-cloud-run-neon-vertex-setup.md`
- `docs/backend/13-staging-runbook.md`
- `docs/backend/14-staging-validation.md`
- `docs/backend/15-staging-without-gcp.md`
- `docs/backend/16-beta-hardening.md`
- `docs/backend/17-offline-sync.md`
- `scripts/staging-preflight.ps1`
- `docs/backend/09-safety-evaluation-cost.md`
- `docs/shared/openapi.yaml`
- `docs/shared/04-database-design.md`
- External mobile docs:
  `C:\Users\Ridwan\Documents\Default Project\sela-documentation\sela-mobile\docs\flutter`
