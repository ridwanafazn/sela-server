# Staging Without GCP

Date: 2026-11-21
Status: local staging alternative; not a production deployment.

This path removes the Google Cloud dependency for staging:

- API, worker, and scheduler run in Docker on the local machine.
- PostgreSQL remains on Neon.
- Text generation uses OpenRouter `deepseek/deepseek-v4.1-flash-1017`.
- Embeddings use Ollama `nomic-embed-text` at 768 dimensions.
- HTTPS is provided by an ngrok tunnel for mobile testing.

It is suitable for staging and Android validation. It is not suitable for a
public beta because the local machine must stay online and ngrok URLs are not a
production ingress.

## Architecture

```text
Flutter / Android
      |
      | HTTPS (ngrok)
      v
Local Docker
  +-- sela-api        :18080
  +-- sela-worker
  +-- sela-scheduler
  +-- ollama/nomic-embed-text
      |
      v
Neon PostgreSQL (staging)
      |
      +-- OpenRouter deepseek/deepseek-v4.1-flash-1017
```

## What Ollama Embedding Means

Ollama converts text into a numeric vector (an embedding) so SELA can retrieve
records by meaning instead of only exact keywords. It does **not** generate
chat responses. Chat responses still come from OpenRouter
`deepseek/deepseek-v4.1-flash-1017`.

The selected model is `nomic-embed-text`:

- about 137M parameters,
- 768 output dimensions, matching the existing database schema,
- quantized model size roughly 250–300 MB,
- CPU inference is sufficient for short journal chunks.

Estimated local footprint:

- Docker image: roughly 1.5–2 GB.
- Model: roughly 250–300 MB.
- Loaded model memory: roughly 0.5–1.5 GB.
- Idle container memory: a few hundred MB.
- CPU: burst usage during embedding/backfill, then idle.

A Ryzen 5 6600H with 16 GB DDR5 and an NVMe drive is comfortably sufficient.
The heavy LLM is not running locally; only the small embedding model is local.
If the machine is used for other work, run the initial backfill when idle.

### Fallback: host Ollama

If the Docker image pull is slow or blocked:

1. Install Ollama for Windows.
2. Run `ollama pull nomic-embed-text`.
3. Set `EMBEDDING_BASE_URL=http://host.docker.internal:11434/v1` in
   `.env.staging.local`.
4. Start only the app services:

```powershell
docker compose -f compose.staging.yml up -d api worker scheduler
```

The API, worker, and scheduler already declare `host.docker.internal` support.

## Verified on 2026-11-21

This path was tested end to end against Neon staging with real providers:

- OpenRouter model: `deepseek/deepseek-v4.1-flash-1017` only.
- Ollama embedding model: `nomic-embed-text`, 768 dimensions.
- Three journals produced three 768-dimensional embeddings.
- Memory extraction produced 8 candidates.
- Companion returned a real DeepSeek response.
- Insight job completed with `insight_created`.
- Schedule and consent lifecycle passed.
- Account deletion and worker purge passed.
- The ngrok health check returned `ok`.
- Usage events recorded real token counts and estimated cost.

The test machine was a Ryzen 5 6600H with 16 GB DDR5 and an NVMe drive. The
embedding workload was not a bottleneck; the cloud LLM carries the heavy text
generation.

## Prerequisites

- Docker Desktop.
- Ollama for Windows installed. The init script uses host Ollama by default.
- `.env.staging` with:
  - `DATABASE_URL`
  - `DATABASE_URL_UNPOOLED`
  - `JWT_ACCESS_SECRET`
  - `RATE_LIMIT_SECRET`
  - `AI_API_KEY`
- ngrok installed and configured.
- The reviewed crisis resource file, or an explicit decision to use the
  embedded defaults for non-real test data only.

The optional container Ollama profile exists in `compose.staging.yml` for
machines where Docker Hub is reachable, but host Ollama is the tested path.

## Setup

1. Create the local staging env file:

```powershell
.\scripts\staging-local-init.ps1
```

This creates `.env.staging.local` and forces:

```text
AI_MODEL=deepseek/deepseek-v4.1-flash-1017
EMBEDDING_PROVIDER=openai
EMBEDDING_BASE_URL=http://ollama:11434/v1
EMBEDDING_MODEL=nomic-embed-text
EMBEDDING_DIMENSION=768
```

2. Start Ollama and pull the embedding model:

```powershell
.\scripts\staging-local.ps1 init
```

3. Apply migrations to Neon staging:

```powershell
.\scripts\staging-local.ps1 migrate
```

4. Start the API, worker, scheduler, and Ollama:

```powershell
.\scripts\staging-local.ps1 up
```

5. Run the smoke test:

```powershell
.\scripts\staging-local.ps1 smoke
```

6. Expose the API to the Android device:

```powershell
.\scripts\staging-local.ps1 tunnel
```

Use the printed HTTPS URL plus `/api/v1` as `API_BASE_URL`.

7. Stop everything:

```powershell
.\scripts\staging-local.ps1 down
```

## Scheduled insights

The `sela-scheduler` container runs a fan-out every 5 minutes. No Cloud
Scheduler is required.

To trigger one fan-out manually:

```powershell
.\scripts\staging-local.ps1 scheduler
```

## Model policy

Staging and production reject any OpenAI-compatible model other than:

```text
deepseek/deepseek-v4.1-flash-1017
```

This is enforced at configuration load time. Use only that model until a new
decision is recorded.

## Switching embeddings to Vertex later

When GCP billing is active:

1. Change `.env.staging.local`:
   - `EMBEDDING_PROVIDER=vertex`
   - `VERTEX_PROJECT_ID=sela-513807`
   - `VERTEX_REGION=asia-southeast1`
   - `VERTEX_EMBEDDING_MODEL=gemini-embedding-001`
2. Restart the containers.
3. Re-enqueue embeddings:

```powershell
docker compose -f compose.staging.yml run --rm `
  --entrypoint /usr/local/bin/sela-worker api -backfill
```

4. Let the worker process the jobs.

## Limitations

- The local machine must remain on and connected.
- A free ngrok URL can change after restart unless a reserved domain is used.
- Local Docker has no Cloud Run health management, autoscaling, or spend caps.
- Ollama embedding quality and speed differ from Vertex.
- The API is publicly reachable through the tunnel. Use only synthetic data
  during staging.
- This path does not replace the production decision; complete billing before a
  public beta.

## Security notes

- `.env.staging.local` is ignored by Git. Do not commit it.
- Do not share the ngrok URL publicly.
- Use synthetic journal data until the crisis resources are reviewed.
- Rotate the OpenRouter key if it is ever exposed.
