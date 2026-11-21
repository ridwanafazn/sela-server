# Session Handoff

Last updated: 2026-11-21 (product vision reconciliation)

## Current state

- Repository: `C:\Users\Ridwan\Git-v2\sela-server`
- Branch: `main`, synced with `origin/main` before local commits
- Latest local commits:
  - `f4d0da1` — staging runbook and Neon free-tier notes
  - `ada9f8d` — local verification handoff
  - a local test-harness commit on top
- Working tree: B1 backend addendum and documentation changes are uncommitted.
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
integration, testing, freeze addendum, and focus phase were updated. Code,
schema, OpenAPI, and production prompts were intentionally not changed.

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

Blocked:

- Billing is disabled for `sela-513807`.
- Cloud Run, Artifact Registry, Cloud Build, Secret Manager, Cloud Scheduler,
  and Vertex AI APIs are disabled.
- `.env.staging` still needs `AI_API_KEY`. `VERTEX_PROJECT_ID` is now set to
  `sela-513807` locally.
- `SAFETY_RESOURCES_FILE` is empty.

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
6. Offline-first - deferred.

The immediate blocker is billing for `sela-513807`. The staging runbook
(`docs/backend/13-staging-runbook.md`) is ready to execute once billing is
active.

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
- The docs package ships OpenAPI 1.2.0; the backend repository is 1.3.0.
  Use the repository contract until the documentation is reconciled.

## Environment status

All project dev services are stopped at handoff:

- `sela-api-smoke` and `sela-mock-llm` containers removed.
- `sela-server-postgres-1` stopped and removed via `docker compose down`.
- No process listening on ports `8080`, `8090`, `5432`, or `9090`.
- PostgreSQL data volume retained:
  `sela-server_postgres-data`

To resume:

```powershell
Set-Location "C:\Users\Ridwan\Git-v2\sela-server"
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
- Embeddings: `fake`.
- Database: local Docker PostgreSQL with pgvector.
- Safety resources: embedded defaults, flagged as unreviewed.
- `.env.staging` (gitignored): Neon pooled/direct URLs and generated JWT and
  rate-limit secrets. `AI_API_KEY` and `VERTEX_PROJECT_ID` are still empty.
- gcloud account is authenticated and the active project is `sela-513807`.

Before staging or real journal use:

- switch the LLM to OpenRouter,
- switch embeddings to Vertex,
- configure reviewed crisis resources,
- configure AI pricing and budgets.

## Selected next-phase architecture

- API: Google Cloud Run
- Database: Neon PostgreSQL with pgvector
- Embeddings: Vertex AI `gemini-embedding-001` at 768 dimensions
- Text generation: OpenRouter `deepseek/deepseek-v4.1-flash-1017`
- Target device: Samsung Galaxy A35 (Android)
- Data: real journal entries

Setup guide:

`docs/backend/12-cloud-run-neon-vertex-setup.md`

Worker options in that guide:

1. Cloud Run Job + Cloud Scheduler every 1–2 minutes — cheapest, 0–2 minutes
   asynchronous latency.
2. Cloud Run worker pool with one instance — near-immediate processing, roughly
   $26–31/month after free-tier usage.

## What the next session needs

### Accounts and access

1. Google Cloud project with billing enabled.
2. Google Cloud CLI installed and authenticated:
   `gcloud auth login`
3. Neon project in `aws-ap-southeast-1` (Singapore) with pooled and direct URLs.
4. OpenRouter account, paid credits, key spend limit, and ZDR/no-training
   privacy settings.
5. A named reviewer for Indonesian crisis resources.
6. Mobile repository / Flutter project for the Android app.

### Credentials

Do not send secrets in chat or commit them. Put values in:

- `.env` locally, and/or
- Google Secret Manager for staging.

Required staging secrets:

- pooled Neon URL,
- direct Neon URL (migrations only),
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

1. Activate billing and link it to `sela-513807`.
2. Create the $1 Cloud Run and Vertex spend caps plus a project-wide alert.
3. Fill `AI_API_KEY` and `VERTEX_PROJECT_ID` in `.env.staging`.
4. Execute `docs/backend/13-staging-runbook.md`.
5. Run the staging smoke test with 3 synthetic entries:
   `scripts/local-smoke.ps1 -BaseUrl <staging>/api/v1 -WorkerMode GCloud -JournalCount 3`.
6. Verify logs, backup/restore, and account purge.
7. Review crisis resources before entering real journal data.
8. Configure the staging Cloud Scheduler trigger for `sela-worker -schedules`.
9. Run the staging smoke test for schedule enable/disable and consent
   grant/revoke.
10. Build the Android Flutter MVP against the staging API, including memory
    review, insight review, schedule controls, consent UI, and editable profile
    preferences.

## Known gaps

- Mobile app is in `C:\Users\Ridwan\Git-v2\sela-mobile`. Phase 1 code is
  complete, but staging and Android-device validation are pending.
- Mobile memory review, insight review, schedule controls, sensitive memory
  consent, editable profile preferences, and the guided writing prompt set are
  not implemented.
- Scheduled insights are implemented in the backend. The staging scheduler
  trigger and mobile controls are pending.
- Per-entry journal analysis is post-MVP and not implemented.
- Sensitive memory consent is implemented in the backend. The mobile consent UI
  is pending.
- Prompt v2 is active. Live model quality evaluation remains manual.
- Staging is not deployed. Billing and the required GCP APIs are disabled.
- `.env.staging` still needs `AI_API_KEY`; `VERTEX_PROJECT_ID` is set locally.
- `GET /journal-changes` and offline-first sync are Phase 3.
- Local health gates, AI usage report, backup/restore verification, and incident
  runbook are implemented. Hosted dashboards/alerts are deferred with GCP.
- Live model quality evaluation for tone, facts, and hallucination is manual.
- Embedded crisis resources need local clinical review.
- Real semantic retrieval still needs Vertex or Ollama plus `make backfill`.
- OpenRouter handles text generation outside GCP. If all journal text must stay
  in GCP, a Vertex Gemini LLM adapter is required and not yet implemented.
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
- `scripts/staging-preflight.ps1`
- `docs/backend/09-safety-evaluation-cost.md`
- `docs/shared/openapi.yaml`
- `docs/shared/04-database-design.md`
- External mobile docs:
  `C:\Users\Ridwan\Documents\Default Project\sela-documentation\sela-mobile\docs\flutter`
