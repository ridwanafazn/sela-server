# Session Handoff

Last updated: 2026-11-16 (local verification session)

## Current state

- Repository: `C:\Users\Ridwan\Git-v2\sela-server`
- Branch: `main`, synced with `origin/main` before local commits
- Latest local commits:
  - `f4d0da1` — staging runbook and Neon free-tier notes
  - this handoff commit
- Release tags:
  - `v0.2.0` — Phase 2 backend freeze
  - `v0.2.1` — Vertex 768-dimensional embedding compatibility
- API contract: OpenAPI `1.3.0`
- Database schema: migration version `16`
- Backend Phase 2: complete and frozen (`10/10` internal stages)

## Pre-Beta position

Workstreams from `docs/backend/11-focus-phase.md`:

1. Real embeddings and live evaluation — not started.
2. Staging deployment — in progress. Neon schema is applied; GCP is blocked on
   an active billing account.
3. Mobile MVP — not started.
4. Beta hardening — not started.
5. Offline-first — deferred.

The immediate blocker is billing for `sela-513807`. The staging runbook
(`docs/backend/13-staging-runbook.md`) is ready to execute once billing is
active.

## Environment status

At the time of this update, local verification services are running:

- `sela-server-postgres-1` on port `5432`.
- `sela-api-smoke` on port `8080` with `APP_ENV=production` and fake AI
  providers. This container is disposable.

Stop them with:

```powershell
docker rm -f sela-api-smoke
docker compose down
```

The PostgreSQL data volume is retained:
`sela-server_postgres-data`.

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
- Container smoke test `bin/local-smoke.ps1`:
  health, register, journals, worker, memories, conversation, companion,
  insight accept, account delete, purge, login blocked.
- API logs contain no journal or message content.
- Neon schema dump (53 KB) restored locally: version 16, 18 tables,
  pgvector 0.8.7.

The insight job fails with the fake provider because it returns a fixed memory
JSON payload; the integration suite covers the real insight path.

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
   `bin/local-smoke.ps1 -BaseUrl <staging>/api/v1 -WorkerMode GCloud -JournalCount 3`.
6. Verify logs, backup/restore, and account purge.
7. Review crisis resources before entering real journal data.
8. Build the Android Flutter MVP against the staging API.

## Known gaps

- Mobile app is a separate repository and not started.
- Staging is not deployed.
- `GET /journal-changes` and offline-first sync are Phase 3.
- Metrics dashboards, alerts, backup automation, and incident runbook are Phase 3.
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
- `docs/backend/10-phase-2-freeze.md`
- `docs/backend/11-focus-phase.md`
- `docs/backend/12-cloud-run-neon-vertex-setup.md`
- `docs/backend/13-staging-runbook.md`
- `docs/backend/09-safety-evaluation-cost.md`
- `docs/shared/openapi.yaml`
- `docs/shared/04-database-design.md`
