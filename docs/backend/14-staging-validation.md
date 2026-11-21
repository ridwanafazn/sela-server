# Staging Validation

Date: 2026-11-21
Project: `sela-513807`
Neon project: `raspy-river-82693559`
Neon region: `aws-ap-southeast-1`
Database: `sela`
Schema: migration version `17`

## Verified

- Neon staging migrated from version `16` to `17`.
- New tables exist:
  - `insight_schedules`
  - `memory_consents`
- `personal_memories.sensitivity` exists.
- Local API binary ran against Neon staging with fake AI providers.
- Smoke test passed:
  - health live and ready,
  - registration,
  - insight schedule enable/disable,
  - sensitive memory consent grant/revoke,
  - journal create and list,
  - worker processing,
  - Companion reply,
  - insight request,
  - account deletion,
  - purge,
  - deleted account cannot log in.
- Smoke logs contained no journal or message content.
- Container image `sela-server:b2-local` built with the B1 backend.

## Non-GCP Alternative

A local staging path is available while GCP billing is blocked:

- Docker API, worker, and scheduler.
- Neon staging database.
- OpenRouter `deepseek/deepseek-v4.1-flash-1017`.
- Ollama `nomic-embed-text` at 768 dimensions.
- ngrok for the public HTTPS URL.

This path was verified end to end on 2026-11-21:

- three 768-dimensional embeddings,
- eight memory candidates,
- a real DeepSeek Companion reply,
- an insight with `insight_created`,
- schedule and consent lifecycle,
- account deletion and purge,
- ngrok health check.

See `15-staging-without-gcp.md`.

## Current Blockers

1. Billing is disabled for `sela-513807`. Both visible billing accounts are
   closed. An API-enable attempt on 2026-11-21 failed with
   `UREQ_PROJECT_BILLING_NOT_FOUND`.
2. Required APIs are disabled:
   - `run.googleapis.com`
   - `artifactregistry.googleapis.com`
   - `cloudbuild.googleapis.com`
   - `secretmanager.googleapis.com`
   - `cloudscheduler.googleapis.com`
   - `aiplatform.googleapis.com`
3. `.env.staging` still requires:
   - `AI_API_KEY` (currently empty).
   - `VERTEX_PROJECT_ID` (local value is now `sela-513807`; it still must be
     provided to the deployed service environment).
4. `SAFETY_RESOURCES_FILE` is empty. Embedded crisis resources remain
   unreviewed.
5. Cloud Run, Cloud Scheduler, Secret Manager, and Vertex cannot be configured
   until billing and the APIs are active.

## Required Manual Actions

1. Open an active billing account and link it to `sela-513807`.
2. Enable the required APIs with `scripts/staging-preflight.ps1` showing zero
   failures.
3. Put the OpenRouter key in `.env.staging` as `AI_API_KEY`.
4. Set `VERTEX_PROJECT_ID=sela-513807` in `.env.staging`.
5. Provide a reviewed Indonesian crisis-resource file and set
   `SAFETY_RESOURCES_FILE`.
6. Confirm the OpenRouter key has a spend limit and no-training/ZDR settings.
7. Confirm Vertex AI permissions for the runtime service account.

## Next After Unblock

1. Run `scripts/staging-preflight.ps1` until it reports zero failures.
2. Create the runtime service account and IAM bindings from
   `12-cloud-run-neon-vertex-setup.md`.
3. Push the image to Artifact Registry.
4. Create Secret Manager entries.
5. Run the migration job.
6. Deploy the API and worker.
7. Deploy the scheduler job and Cloud Scheduler trigger for
   `sela-worker -schedules`.
8. Run `scripts/local-smoke.ps1` with `-WorkerMode GCloud` against the staging
   URL.
9. Verify logs, backup/restore, account purge, AI budgets, and crisis resources.
10. Run the live model evaluation with Vertex and OpenRouter.
