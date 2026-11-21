# Cloud Run + Neon + Vertex Setup

Target architecture:

```text
Samsung Galaxy A35 (Flutter)
          |
          | HTTPS
          v
Cloud Run service: sela-api
          |
          +-- Neon Postgres + pgvector
          +-- Vertex AI embeddings (gemini-embedding-001, 768 dims)
          +-- OpenRouter (DeepSeek V4.1 Flash)

Cloud Run worker: sela-worker
          |
          +-- Neon Postgres + pgvector
          +-- Vertex AI embeddings
          +-- OpenRouter
```

The mobile client never receives provider keys or database credentials.

## 1. Accounts and decisions

### Required accounts

- Google Cloud project with billing enabled.
- Neon account and project.
- OpenRouter account with credits and a key spend limit.
- GitHub/repository access for the deployment image.
- A named person to review Indonesian crisis resources.

### Optional

- Custom domain. Cloud Run provides an HTTPS `*.run.app` URL without one.
- Google Play Console only when distributing through Play Store. For personal
  use on Android, an APK can be sideloaded.

### Recommended regions

- Cloud Run API: `asia-southeast2` (Jakarta) or `asia-southeast1` (Singapore).
- Cloud Run worker: same region as the API and database where possible.
- Neon: `aws-ap-southeast-1` (Singapore).
- Vertex: `asia-southeast1` if the selected model is available there, otherwise
  a supported nearby region.

## 2. Google Cloud project

```bash
export PROJECT_ID="your-project-id"
export REGION="asia-southeast2"
export VERTEX_REGION="asia-southeast1"
export AR_REPO="sela"
export IMAGE="$REGION-docker.pkg.dev/$PROJECT_ID/$AR_REPO/sela-server:v0.2.1"

gcloud config set project "$PROJECT_ID"

gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com \
  aiplatform.googleapis.com \
  logging.googleapis.com \
  monitoring.googleapis.com
```

Create the Artifact Registry repository:

```bash
gcloud artifacts repositories create "$AR_REPO" \
  --repository-format=docker \
  --location="$REGION"
```

Create a runtime service account:

```bash
gcloud iam service-accounts create sela-runtime \
  --display-name="SELA runtime"

export RUNTIME_SA="sela-runtime@$PROJECT_ID.iam.gserviceaccount.com"

for ROLE in \
  roles/aiplatform.user \
  roles/secretmanager.secretAccessor \
  roles/logging.logWriter; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:$RUNTIME_SA" \
    --role="$ROLE"
done
```

## 3. Neon

1. Create a project in `aws-ap-southeast-1` (Singapore).
2. Create database `sela`.
3. Copy both connection strings:
   - **pooled** for the API and worker (`-pooler` in the host),
   - **direct** for migrations and `pg_dump`.
4. Add `sslmode=require` if the copied URL does not include it.
5. Run migrations with the direct URL.

The migration runner creates the `vector` extension. To verify:

```sql
CREATE EXTENSION IF NOT EXISTS vector;
SELECT extname, extversion FROM pg_extension WHERE extname = 'vector';
```

Neon free computes suspend after inactivity. The first request after a quiet
period can take a few hundred milliseconds longer.

## 4. Build and push the image

```bash
gcloud auth configure-docker "$REGION-docker.pkg.dev"

docker build --build-arg VERSION=v0.2.1 -t "$IMAGE" .
docker push "$IMAGE"
```

The image contains `sela-api`, `sela-migrate`, and `sela-worker`.

## 5. Secret Manager

Create secrets in the Google Cloud console or with `gcloud`. Do not commit
secret values.

Required:

- `sela-database-url` — Neon pooled URL.
- `sela-database-url-unpooled` — Neon direct URL, migrations only.
- `sela-jwt-access-secret` — at least 32 random bytes.
- `sela-rate-limit-secret` — at least 32 random bytes.
- `sela-openrouter-api-key`.

Optional:

- `sela-safety-resources` — reviewed crisis resource JSON.
- `sela-vertex-service-account` — only if you use a dedicated key instead of the
  Cloud Run service account. ADC is preferred.

Example creation with a local file:

```bash
gcloud secrets create sela-database-url --data-file=database-url.txt
gcloud secrets create sela-database-url-unpooled --data-file=database-url-direct.txt
gcloud secrets create sela-jwt-access-secret --data-file=jwt-secret.txt
gcloud secrets create sela-rate-limit-secret --data-file=rate-limit-secret.txt
gcloud secrets create sela-openrouter-api-key --data-file=openrouter-key.txt
```

Delete the local secret files after uploading.

## 6. Run migrations

Run the migration job before deploying the API:

```bash
gcloud run jobs deploy sela-migrate \
  --image="$IMAGE" \
  --region="$REGION" \
  --service-account="$RUNTIME_SA" \
  --command=/usr/local/bin/sela-migrate \
  --args=up \
  --max-retries=0 \
  --set-secrets=DATABASE_URL_UNPOOLED=sela-database-url-unpooled:latest

gcloud run jobs execute sela-migrate --region="$REGION" --wait
```

Check:

```bash
gcloud run jobs executions list --job=sela-migrate --region="$REGION"
```

## 7. Deploy the API

Non-secret environment:

```text
APP_ENV=production
APP_NAME=sela-server
APP_PORT=8080
LOG_LEVEL=info
LOG_FORMAT=json
JWT_ISSUER=sela-api
JWT_AUDIENCE=sela-mobile
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h
RATE_LIMIT_TRUSTED_PROXY_HOPS=1
AI_PROVIDER=openai
AI_BASE_URL=https://openrouter.ai/api/v1
AI_MODEL=deepseek/deepseek-v4.1-flash-1017
AI_TIMEOUT=120s
AI_MAX_RETRIES=3
EMBEDDING_PROVIDER=vertex
VERTEX_PROJECT_ID=your-project-id
VERTEX_REGION=asia-southeast1
VERTEX_EMBEDDING_MODEL=gemini-embedding-001
EMBEDDING_DIMENSION=768
AI_INPUT_COST_PER_MILLION=0.15
AI_OUTPUT_COST_PER_MILLION=0.60
AI_EMBEDDING_COST_PER_MILLION=0
AI_USER_MONTHLY_BUDGET_USD=5
AI_GLOBAL_MONTHLY_BUDGET_USD=20
```

Verify current OpenRouter pricing and Vertex pricing before relying on the
budget values. `0` disables a budget or price.

Deploy:

```bash
gcloud run deploy sela-api \
  --image="$IMAGE" \
  --region="$REGION" \
  --platform=managed \
  --allow-unauthenticated \
  --service-account="$RUNTIME_SA" \
  --min-instances=0 \
  --max-instances=2 \
  --cpu=1 \
  --memory=512Mi \
  --timeout=120 \
  --set-env-vars=APP_ENV=production,APP_NAME=sela-server,APP_PORT=8080,LOG_LEVEL=info,LOG_FORMAT=json,JWT_ISSUER=sela-api,JWT_AUDIENCE=sela-mobile,JWT_ACCESS_TTL=15m,JWT_REFRESH_TTL=720h,RATE_LIMIT_TRUSTED_PROXY_HOPS=1,AI_PROVIDER=openai,AI_BASE_URL=https://openrouter.ai/api/v1,AI_MODEL=deepseek/deepseek-v4.1-flash-1017,AI_TIMEOUT=120s,AI_MAX_RETRIES=3,EMBEDDING_PROVIDER=vertex,VERTEX_PROJECT_ID="$PROJECT_ID",VERTEX_REGION="$VERTEX_REGION",VERTEX_EMBEDDING_MODEL=gemini-embedding-001,EMBEDDING_DIMENSION=768,AI_INPUT_COST_PER_MILLION=0.15,AI_OUTPUT_COST_PER_MILLION=0.60,AI_EMBEDDING_COST_PER_MILLION=0,AI_USER_MONTHLY_BUDGET_USD=5,AI_GLOBAL_MONTHLY_BUDGET_USD=20 \
  --set-secrets=DATABASE_URL=sela-database-url:latest,JWT_ACCESS_SECRET=sela-jwt-access-secret:latest,RATE_LIMIT_SECRET=sela-rate-limit-secret:latest,AI_API_KEY=sela-openrouter-api-key:latest
```

Cloud Run provides HTTPS automatically. Note the service URL.

## 8. Deploy the worker

The worker must run continuously or be triggered frequently.

### Option A — Cloud Run worker pool

A worker pool is the most direct option. It has no HTTP endpoint and must have
at least one instance. A 1 vCPU / 512 MiB worker pool is roughly $25–31 per
month after free-tier usage; verify current pricing.

```bash
gcloud run worker-pools deploy sela-worker \
  --image="$IMAGE" \
  --region="$REGION" \
  --service-account="$RUNTIME_SA" \
  --instances=1 \
  --cpu=1 \
  --memory=512Mi \
  --command=/usr/local/bin/sela-worker \
  --set-env-vars=APP_ENV=production,APP_NAME=sela-server,LOG_LEVEL=info,LOG_FORMAT=json,AI_PROVIDER=openai,AI_BASE_URL=https://openrouter.ai/api/v1,AI_MODEL=deepseek/deepseek-v4.1-flash-1017,AI_TIMEOUT=120s,AI_MAX_RETRIES=3,EMBEDDING_PROVIDER=vertex,VERTEX_PROJECT_ID="$PROJECT_ID",VERTEX_REGION="$VERTEX_REGION",VERTEX_EMBEDDING_MODEL=gemini-embedding-001,EMBEDDING_DIMENSION=768,AI_INPUT_COST_PER_MILLION=0.15,AI_OUTPUT_COST_PER_MILLION=0.60,AI_EMBEDDING_COST_PER_MILLION=0,AI_USER_MONTHLY_BUDGET_USD=5,AI_GLOBAL_MONTHLY_BUDGET_USD=20 \
  --set-secrets=DATABASE_URL=sela-database-url:latest,AI_API_KEY=sela-openrouter-api-key:latest
```

### Option B — Cloud Run Job every 1–2 minutes

This is cheaper for a personal deployment. It adds up to 1–2 minutes of queue
latency for embeddings, memory extraction, insights, and account purge.

Scheduled insights are implemented in the worker through the `-schedules` flag.
Configure Cloud Scheduler or an equivalent trigger to run
`sela-worker -schedules` every few minutes. The flag enqueues due weekly/monthly
insights; the normal worker then processes them.

```bash
gcloud run jobs deploy sela-worker \
  --image="$IMAGE" \
  --region="$REGION" \
  --service-account="$RUNTIME_SA" \
  --command=/usr/local/bin/sela-worker \
  --args=-once \
  --max-retries=0 \
  --task-timeout=600 \
  --set-env-vars=APP_ENV=production,APP_NAME=sela-server,LOG_LEVEL=info,LOG_FORMAT=json,AI_PROVIDER=openai,AI_BASE_URL=https://openrouter.ai/api/v1,AI_MODEL=deepseek/deepseek-v4.1-flash-1017,AI_TIMEOUT=120s,AI_MAX_RETRIES=3,EMBEDDING_PROVIDER=vertex,VERTEX_PROJECT_ID="$PROJECT_ID",VERTEX_REGION="$VERTEX_REGION",VERTEX_EMBEDDING_MODEL=gemini-embedding-001,EMBEDDING_DIMENSION=768,AI_INPUT_COST_PER_MILLION=0.15,AI_OUTPUT_COST_PER_MILLION=0.60,AI_EMBEDDING_COST_PER_MILLION=0,AI_USER_MONTHLY_BUDGET_USD=5,AI_GLOBAL_MONTHLY_BUDGET_USD=20 \
  --set-secrets=DATABASE_URL=sela-database-url:latest,AI_API_KEY=sela-openrouter-api-key:latest
```

Trigger it with Cloud Scheduler every 1–2 minutes. The exact Scheduler command
depends on whether you create an HTTP target to the Cloud Run Jobs API or use
the console; configure an OAuth service account with permission to execute the
job.

Start with Option A if you want immediate processing. Move to Option B if the
always-on cost matters.

### Pre-beta cost constraint (Neon Free)

Neon Free includes 100 CU-hours per month on a 0.25 CU compute, which is about
400 active hours. A worker that polls every 1–2 minutes keeps the database
awake around the clock (about 730 hours, or 182 CU-hours per month), so Neon
suspends the compute mid-month.

For pre-beta staging, run the worker on demand instead of scheduling it:

```bash
gcloud run jobs execute sela-worker --region="$REGION" --wait
```

Enable Cloud Scheduler only for limited testing windows, or upgrade the Neon
plan before running an always-on worker. A $1 monthly spend cap on Cloud Run
and another on Vertex AI limits staging exposure; spend caps pause new usage
at 100% but are not instantaneous, so small overages remain possible.

## 9. OpenRouter privacy

For real journal content:

- Use a paid key, not free promotional models.
- Set a monthly key spend limit.
- In OpenRouter Privacy settings, disable optional prompt/completion logging
  and data-sharing discounts.
- Enforce ZDR for the model group serving DeepSeek, or set provider
  `data_collection` / ZDR policy to deny training and unnecessary retention.
- Verify the selected provider endpoint supports the required policy.
- Remember that OpenRouter is outside GCP; journal text leaves Google Cloud for
  text generation. Vertex is used only for embeddings in this architecture.

If all journal text must stay in GCP, a Vertex Gemini LLM adapter is required.
That adapter is not implemented yet.

## 10. Mobile configuration

The Flutter app needs:

```text
API_BASE_URL=https://sela-api-xxxxx.run.app/api/v1
```

Use HTTPS only. Do not enable Android cleartext traffic for production.

For personal use on the Galaxy A35:

- Build a release APK.
- Sideload it.
- Use secure storage for access and refresh tokens.
- Clear local data after logout or account deletion.

A Play Store release is not required for personal use. A Play Console account is
only needed for closed testing or distribution.

## 11. Smoke test

```bash
API_URL="https://sela-api-xxxxx.run.app/api/v1"

curl "$API_URL/health/live"
curl "$API_URL/health/ready"

curl -X POST "$API_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"..."}'
```

Then test:

1. login,
2. create a journal,
3. wait for worker processing,
4. list memory candidates,
5. send a Companion message,
6. request an insight and poll the job,
7. submit feedback,
8. delete the account and verify purge.

Check logs:

```bash
gcloud logging read \
  'resource.type="cloud_run_revision" AND resource.labels.service_name="sela-api"' \
  --limit=50

gcloud logging read \
  'resource.type="cloud_run_worker_pool" AND resource.labels.worker_pool_name="sela-worker"' \
  --limit=50
```

Logs must not contain journal or message content.

## 12. Backups

- Configure a Neon backup/restore policy appropriate to the plan.
- Run a periodic `pg_dump` using the direct connection.
- Store encrypted dumps outside the database account.
- Verify one restore before entering real journal data.

## 13. Rollback

- Deploy a previous image tag.
- Migrations are versioned; do not roll back a migration unless the previous
  image is compatible.
- Keep the previous image tag available in Artifact Registry.
