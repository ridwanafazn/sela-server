# SELA Backend Documentation

This directory is the canonical backend documentation set. The mobile
repository keeps read-only snapshots for development; see
`C:\Users\Ridwan\Git-v2\sela-mobile\docs\backend\README.md`.

## Current baseline

| Item | Value |
|---|---|
| API contract | OpenAPI `1.7.0` |
| Database schema | migration `21` |
| Backend Phase 2 | frozen at `v0.2.0` |
| Phase 4A | per-entry journal analysis backend complete |
| Phase 4B | privacy center and unified search backend complete |
| Verified staging | local Docker + Neon + OpenRouter + Ollama |
| GCP | deferred until billing and APIs are enabled |

## Source of truth

Use this order when documents disagree:

1. `docs/shared/openapi.yaml` for routes and schemas.
2. `docs/shared/04-database-design.md` and migrations for persistence.
3. `docs/shared/06-api-contract.md` for behavioral API rules.
4. `docs/shared/01-product-specification.md` and
   `docs/shared/10-product-vision-decisions.md` for product scope.
5. `docs/backend/*` for implementation and operations.
6. `HANDOFF.md` for the current session state.
7. `README.md` for the project overview.

## Shared documents

| File | Purpose |
|---|---|
| `shared/01-product-specification.md` | Product scope, modules, functional requirements |
| `shared/02-product-frameworks.md` | Internal reflection frameworks and usage rules |
| `shared/03-system-architecture.md` | Modular monolith and AI pipeline |
| `shared/04-database-design.md` | Tables, constraints, indexes, deferred schema |
| `shared/05-ai-system-design.md` | Prompts, retrieval, memory, insights, privacy |
| `shared/06-api-contract.md` | API behavior, errors, idempotency, phase rules |
| `shared/07-security-and-privacy.md` | Data classes, consent, deletion, logging |
| `shared/08-operations-and-reliability.md` | Environments, jobs, monitoring, backups |
| `shared/09-roadmap.md` | Product and engineering phases |
| `shared/10-product-vision-decisions.md` | Decision record and implementation status |
| `shared/openapi.yaml` | Machine-readable API contract |
| `shared/database-erd.dbml` | Conceptual ERD |

## Backend documents

| File | Status | Purpose |
|---|---|---|
| `backend/01-development-guide.md` | Current | Stack, package layout, workflow |
| `backend/02-backend-implementation.md` | Current | Implementation stages and acceptance |
| `backend/03-ai-integration.md` | Current | Provider adapters, prompts, retrieval, analysis |
| `backend/04-testing-and-deployment.md` | Current | Test strategy, CI, deployment |
| `backend/09-safety-evaluation-cost.md` | Current | Safety gates, evaluation, cost controls |
| `backend/10-phase-2-freeze.md` | Historical | Freeze at schema `16`, OpenAPI `1.3.0` |
| `backend/11-focus-phase.md` | Current | Pre-beta and depth workstreams |
| `backend/12-cloud-run-neon-vertex-setup.md` | Deferred | GCP deployment runbook |
| `backend/13-staging-runbook.md` | Current | Staging status and GCP runbook |
| `backend/14-staging-validation.md` | Historical | Validation at schema `17` |
| `backend/15-staging-without-gcp.md` | Current | Verified local staging path |
| `backend/16-beta-hardening.md` | Current | Health, usage, backup, incidents |
| `backend/17-offline-sync.md` | Current | `GET /journal-changes` contract |
| `backend/18-prepublic.md` | Current | Verification, reset, sessions, export, consent |
| `backend/19-journal-analysis.md` | Current | Per-entry analysis backend |
| `backend/20-phase-4b.md` | Current | Privacy center, unified search, deferred 4B+ |

## Sync to the mobile repository

The mobile repository consumes read-only snapshots:

```text
C:\Users\Ridwan\Git-v2\sela-mobile\docs\backend\
```

Run from the mobile repository:

```powershell
.\tool\sync_backend_docs.ps1
```

The script copies the contract, database, AI, security, roadmap, pre-public,
analysis, and Phase 4B documents, then writes `docs/backend/SYNC.md` with the
source commit and current versions. Never edit the mobile snapshots directly.

## Validation

```powershell
go test ./...
go test -tags=integration ./...
go vet ./...
npx --yes @redocly/cli@2.78.0 lint docs/shared/openapi.yaml
```

OpenAPI lint is valid with four pre-existing warnings on the health endpoints.

## Historical documents

`backend/10-phase-2-freeze.md` and `backend/14-staging-validation.md` are
snapshots of their dates. They intentionally keep older versions and must not be
used as the current state. Use `README.md`, `HANDOFF.md`, and the current
documents above.
