# Staging Runbook — sela-513807

Status: Neon staging is migrated to schema `21` and passed local API/worker
smoke tests, including real-provider journal analysis, privacy enforcement, and
unified search. Google Cloud deployment has not been executed because billing is
disabled. See `docs/backend/14-staging-validation.md`,
`docs/backend/19-journal-analysis.md`, and `docs/backend/20-phase-4b.md`.

A non-GCP local staging path is documented in
`docs/backend/15-staging-without-gcp.md` while the GCP billing blocker is
resolved.

Decisions:

- Cloud Run region: `asia-southeast1` (Singapore), co-located with Neon and Vertex.
- Neon project: `raspy-river-82693559`, database `sela`, PostgreSQL 17, pgvector 0.8.7.
- Worker: manual Cloud Run Job for async work.
- Scheduler: a low-frequency Cloud Scheduler trigger runs `sela-worker
  -schedules` for weekly/monthly insights. Verify Neon Free CU-hours before
  enabling an always-on worker.
- Image: `asia-southeast1-docker.pkg.dev/sela-513807/sela/sela-server:v0.2.1`.

## Fixed values

```text
PROJECT_ID=sela-513807
REGION=asia-southeast1
VERTEX_REGION=asia-southeast1
AR_REPO=sela
IMAGE=asia-southeast1-docker.pkg.dev/sela-513807/sela/sela-server:v0.2.1
RUNTIME_SA=sela-runtime@sela-513807.iam.gserviceaccount.com
WORKER_JOB=sela-worker
```

## Preconditions

- Billing account linked to `sela-513807`.
- `.env.staging` has `AI_API_KEY` and `VERTEX_PROJECT_ID=sela-513807`.
- `gcloud auth login` completed for the owning account.
- The local Docker image builds: `docker build -t sela-server:v0.2.1-local .`

## 0. Verify billing

```powershell
gcloud billing projects describe sela-513807
```

Expected: `billingEnabled: true`.

## 1. Spend caps and alerts

Spend caps use gross estimated costs and pause new usage at 100%. They are
limited to one project and one eligible service per budget.

Enable the Budget API first:

```powershell
gcloud services enable billingbudgets.googleapis.com --project=sela-513807
```

Create the Cloud Run cap (`152E-C115-5142`), Vertex cap (`C7E2-9256-1C43`), and
a project-wide alerts-only budget. Use the Cloud Billing Budget API; the
installed gcloud SDK does not expose spend-cap flags.

```powershell
$billing = "<BILLING_ACCOUNT_ID>"
$token = gcloud auth print-access-token
$headers = @{
  Authorization          = "Bearer $token"
  "x-goog-user-project"  = "sela-513807"
}
Invoke-RestMethod -Method Post `
  -Uri "https://billingbudgets.googleapis.com/v1/billingAccounts/$billing/budgets" `
  -Headers $headers -ContentType "application/json" -InFile cap-cloud-run.json
```

`cap-cloud-run.json`:

```json
{
  "displayName": "sela cloud run cap",
  "budgetFilter": {
    "projects": ["projects/sela-513807"],
    "services": ["services/152E-C115-5142"],
    "creditTypesTreatment": "EXCLUDE_ALL_CREDITS",
    "calendarPeriod": "MONTH"
  },
  "amount": { "specifiedAmount": { "units": "1", "nanos": 0 } },
  "thresholdRules": [
    { "thresholdPercent": 0.5 },
    { "thresholdPercent": 0.8 },
    { "thresholdPercent": 1.0 }
  ],
  "notificationsRule": { "enableProjectLevelRecipients": true },
  "spendCap": { "inputState": "CONFIGURED" }
}
```

Repeat with `services/C7E2-9256-1C43` for Vertex. Use an alerts-only budget
for the rest of the project at $1.

## 2. Enable APIs

```powershell
gcloud services enable `
  run.googleapis.com `
  artifactregistry.googleapis.com `
  cloudbuild.googleapis.com `
  secretmanager.googleapis.com `
  aiplatform.googleapis.com `
  logging.googleapis.com `
  monitoring.googleapis.com `
  iam.googleapis.com `
  --project=sela-513807
```

## 3. Artifact Registry and runtime service account

```powershell
gcloud artifacts repositories create sela `
  --repository-format=docker `
  --location=asia-southeast1 `
  --project=sela-513807

gcloud iam service-accounts create sela-runtime `
  --display-name="SELA runtime" `
  --project=sela-513807

foreach ($role in @(
  "roles/aiplatform.user",
  "roles/secretmanager.secretAccessor",
  "roles/logging.logWriter"
)) {
  gcloud projects add-iam-policy-binding sela-513807 `
    --member="serviceAccount:sela-runtime@sela-513807.iam.gserviceaccount.com" `
    --role=$role --quiet
}
```

## 4. Secrets

Required secret names:

- `sela-database-url` — Neon pooled URL.
- `sela-database-url-unpooled` — Neon direct URL.
- `sela-jwt-access-secret`.
- `sela-rate-limit-secret`.
- `sela-openrouter-api-key`.

Read the values from `.env.staging`, write temporary files, upload, then delete
the temporary files. Never echo the values.

```powershell
function Set-SecretVersion([string]$name, [string]$value) {
  $tmp = Join-Path $env:TEMP "$name.txt"
  Set-Content -LiteralPath $tmp -Value $value -NoNewline -Encoding utf8
  gcloud secrets describe $name --project=sela-513807 *> $null
  if ($LASTEXITCODE -eq 0) {
    gcloud secrets versions add $name --data-file=$tmp --project=sela-513807
  } else {
    gcloud secrets create $name --data-file=$tmp --project=sela-513807
  }
  Remove-Item -LiteralPath $tmp -Force
}
```

Map `.env.staging` keys to secret names:

```text
DATABASE_URL               -> sela-database-url
DATABASE_URL_UNPOOLED      -> sela-database-url-unpooled
JWT_ACCESS_SECRET          -> sela-jwt-access-secret
RATE_LIMIT_SECRET          -> sela-rate-limit-secret
AI_API_KEY                 -> sela-openrouter-api-key
```

## 5. Build and push the image

```powershell
gcloud auth configure-docker asia-southeast1-docker.pkg.dev

docker build --build-arg VERSION=v0.2.1 -t $IMAGE .
docker push $IMAGE
```

## 6. Run migrations

The migration job uses the direct Neon URL and the embedded migration runner.

```powershell
gcloud run jobs deploy sela-migrate `
  --image=$IMAGE --region=asia-southeast1 --project=sela-513807 `
  --service-account=sela-runtime@sela-513807.iam.gserviceaccount.com `
  --command=/usr/local/bin/sela-migrate --args=up --max-retries=0 `
  --set-secrets=DATABASE_URL_UNPOOLED=sela-database-url-unpooled:latest

gcloud run jobs execute sela-migrate --region=asia-southeast1 --project=sela-513807 --wait
```

Expected: `version=16 dirty=false`. The local dry run already applied this
schema to Neon, so the first cloud run should report no change.

## 7. Deploy the API

Non-secret environment (verify prices before relying on budgets):

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
VERTEX_PROJECT_ID=sela-513807
VERTEX_REGION=asia-southeast1
VERTEX_EMBEDDING_MODEL=gemini-embedding-001
EMBEDDING_DIMENSION=768
AI_INPUT_COST_PER_MILLION=0.038
AI_OUTPUT_COST_PER_MILLION=0.50
AI_EMBEDDING_COST_PER_MILLION=0.15
AI_USER_MONTHLY_BUDGET_USD=1
AI_GLOBAL_MONTHLY_BUDGET_USD=20
```

```powershell
gcloud run deploy sela-api `
  --image=$IMAGE --region=asia-southeast1 --project=sela-513807 `
  --platform=managed --allow-unauthenticated `
  --service-account=sela-runtime@sela-513807.iam.gserviceaccount.com `
  --min-instances=0 --max-instances=2 --cpu=1 --memory=512Mi --timeout=120 `
  --set-env-vars="<the block above as comma-separated KEY=VALUE>" `
  --set-secrets="DATABASE_URL=sela-database-url:latest,JWT_ACCESS_SECRET=sela-jwt-access-secret:latest,RATE_LIMIT_SECRET=sela-rate-limit-secret:latest,AI_API_KEY=sela-openrouter-api-key:latest"
```

Record the service URL. Cloud Run provides HTTPS.

## 8. Deploy the worker job

The schedule preference API and fan-out are implemented. The staging Cloud
Scheduler trigger is not configured yet. For the current staging pass, execute
the worker on demand after each async step and run `sela-worker -schedules` to
test due schedules.

Do not mark automatic scheduled insights as verified until the Cloud Scheduler
trigger is configured.

```powershell
gcloud run jobs deploy sela-worker `
  --image=$IMAGE --region=asia-southeast1 --project=sela-513807 `
  --service-account=sela-runtime@sela-513807.iam.gserviceaccount.com `
  --command=/usr/local/bin/sela-worker --args=-once `
  --max-retries=0 --task-timeout=600 `
  --set-env-vars="<same non-secret block as the API>" `
  --set-secrets="DATABASE_URL=sela-database-url:latest,AI_API_KEY=sela-openrouter-api-key:latest"
```

Run it on demand:

```powershell
gcloud run jobs execute sela-worker --region=asia-southeast1 --project=sela-513807 --wait
```

## 8A. Create the insight scheduler trigger

After billing and the APIs are active:

1. Deploy a second Cloud Run Job named `sela-scheduler` from the same image:
   - command: `/usr/local/bin/sela-worker`
   - args: `-schedules`
   - max retries: `0`
   - task timeout: `300`
2. Create a Cloud Scheduler HTTP target that invokes the Cloud Run Jobs API
   `run` method for `sela-scheduler` with an OAuth service account.
3. Use a low frequency, such as every 5–15 minutes. Verify the Neon Free
   CU-hour impact before enabling it permanently.
4. Verify by setting a test user's `next_run_at` in the past and confirming that
   one `insight.generate` job is created with `generation_type=weekly` or
   `monthly`, and that a second scheduler tick does not duplicate it.

The exact HTTP target URI depends on the Cloud Run Jobs API version and region;
verify it in the Console before creating the Scheduler job.

## 9. Smoke test

```powershell
$api = (gcloud run services describe sela-api --region=asia-southeast1 --project=sela-513807 --format="value(status.url)") + "/api/v1"
.\scripts\local-smoke.ps1 -BaseUrl $api -WorkerMode GCloud -JournalCount 3
```

The script creates a synthetic user, three journals, a conversation, an
insight request, runs the worker after each async step, and deletes the
account. It fails if any step returns an unexpected status.

The current script covers on-demand insights. Extend it to cover schedule
enable/disable, `sela-worker -schedules`, and sensitive memory consent
grant/revoke.

Then verify:

- `GET /api/v1/health/ready` returns database and migrations `ok`.
- Account deletion removes the user and blocks login.
- Logs contain no journal or message content:

```powershell
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="sela-api"' --limit=200 --project=sela-513807
```

Search for a known synthetic sentence; it must not appear.

## 10. Backup and restore

The local dry run already dumped the Neon schema (53 KB) and restored it into
a disposable local database: version 16, 18 tables, pgvector 0.8.7.

For staging:

```powershell
$dsn = (Select-String .env.staging -Pattern '^DATABASE_URL_UNPOOLED=').Line.Substring(21)
docker run --rm -v "${PWD}/bin:/backup" pgvector/pgvector:pg17 `
  pg_dump -d $dsn -Fc -f /backup/neon-staging.dump
```

Store encrypted dumps outside the database account. Verify one restore before
entering real journal data.

## Rollback

- Redeploy the previous image tag.
- Migrations are forward-only; do not roll back unless the previous image is
  compatible with the older schema.
- Keep the previous image tag in Artifact Registry.

## Not yet verifiable in this staging pass

- Cloud Scheduler trigger for `sela-worker -schedules`.
- Live model quality evaluation for prompt v2.
- Mobile memory review, insight review, schedule controls, consent UI, and
  editable profile preferences.

## Out of scope for staging

- Crisis resource review.
- Privacy policy and consent.
- Email verification and password reset.
- Subscription and entitlements.
- `GET /journal-changes` and offline sync.
