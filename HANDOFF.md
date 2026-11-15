# Session Handoff

Last updated: 2026-11-15 (session end)

## Current state

- Repository: `C:\Users\Ridwan\Git-v2\sela-server`
- Branch: `main`, synced with `origin/main`
- Latest commit: `b24f3c3 fix: support Vertex gemini-embedding-001 at 768 dimensions`
- Release tags:
  - `v0.2.0` — Phase 2 backend freeze
  - `v0.2.1` — Vertex 768-dimensional embedding compatibility
- API contract: OpenAPI `1.3.0`
- Database schema: migration version `16`
- Backend Phase 2: complete and frozen (`10/10` internal stages)

## Environment status

All project dev services were stopped at session end:

- `sela-swagger` container removed.
- `sela-server-postgres-1` stopped and removed via `docker compose down`.
- No process listening on ports `8080`, `8090`, or `5432`.
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

## Local configuration

`.env` is local, ignored by Git, and contains the current development
credentials. Do not commit it.

Current local state:

- LLM: OpenCode Go OpenAI-compatible endpoint.
- Model: `deepseek-v4.1-flash`.
- Embeddings: `fake`.
- Database: local Docker PostgreSQL with pgvector.
- Safety resources: embedded defaults, flagged as unreviewed.

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

- Docker: installed.
- Flutter: `3.47.0` installed.
- `gcloud`: not installed.
- `adb`: not on `PATH`.
- Android Studio / Android platform tools still need verification.

## Next steps

1. Install and authenticate Google Cloud CLI.
2. Create the Neon project and copy both connection strings.
3. Create the OpenRouter key and configure privacy/ZDR.
4. Choose Cloud Run Job + Scheduler or a worker pool.
5. Run `docs/backend/12-cloud-run-neon-vertex-setup.md`.
6. Deploy migrations, API, and worker.
7. Run the staging smoke test with 2–3 synthetic entries.
8. Verify backup/restore and account deletion.
9. Only then enter real journal data.
10. Build the Android Flutter MVP against the staging API.

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

## Useful docs

- `CHANGELOG.md`
- `docs/backend/10-phase-2-freeze.md`
- `docs/backend/11-focus-phase.md`
- `docs/backend/12-cloud-run-neon-vertex-setup.md`
- `docs/backend/09-safety-evaluation-cost.md`
- `docs/shared/openapi.yaml`
- `docs/shared/04-database-design.md`
