# SELA Backend Testing and Deployment

Version: 1.3

Status: current backend baseline through Phase 4B

## 1. Purpose

This document defines backend testing, containerization, CI/CD, deployment, and release requirements.

The target GCP deployment is Cloud Run with images stored in Artifact Registry.
The verified staging path is local Docker with Neon PostgreSQL, OpenRouter, and
Ollama; see `docs/backend/15-staging-without-gcp.md`. GCP remains deferred until
billing and the required APIs are enabled.

## 2. Testing Strategy

Testing must cover application behavior, database integration, authorization, API compatibility, and AI provider boundaries.

### Unit Tests

Test:

- Business rules.
- Service behavior.
- Validation.
- Error mapping.
- Memory status transitions.
- Insight processing rules.
- Job retry logic.

Unit tests should use mocks or fakes for external dependencies.

### Integration Tests

Test:

- PostgreSQL persistence.
- Database transactions.
- Migration compatibility.
- Repository queries.
- Authentication.
- Journal ownership.
- Conversation persistence.
- Memory provenance.
- Job claiming and locking.
- Server-side journal drafts and publishing.
- Journal version increment, `If-Match`, and conflict detection.
- Journal change feed including tombstones.
- Insight job creation, status transitions, and evidence persistence.
- Scheduled insight preference persistence, timezone handling, and idempotency.
- Scheduled insight opt-out and budget blocking.
- Scheduled insight `insufficient_evidence` behavior.
- Sensitive memory skipped by default.
- Sensitive memory stored only with explicit consent and excluded after
  revocation.
- Idempotent message and insight requests.
- Account deletion deactivation and purge job creation.
- Refresh-token rotation and reuse detection.
- Email verification, password reset, session inventory, and session revocation.
- Per-entry analysis request, replay, polling, version conflict, stale marker,
  deletion, and journal-delete cleanup.
- Privacy preference defaults, updates, and per-user isolation.
- Companion, insight, and memory-extraction privacy enforcement.
- Unified search across journals, memories, insights, and analyses.
- Privacy preferences in data export.

Use an isolated test database.

### API Tests

Test:

- HTTP status codes.
- Request validation.
- Response structures.
- Authentication middleware.
- Authorization.
- Pagination.
- Error responses and field-error details.
- Idempotency replay and key-reuse conflict.
- Missing `If-Match` returns 428.
- Stale journal version returns 409.
- Journal change feed returns tombstones, drafts, cursor pages, and
  `sync.server_time`.
- Asynchronous insight acceptance and job status.
- Scheduled insight enable, update, read, and disable.
- Scheduled insight schedule changes do not create duplicate jobs.
- Sensitive memory consent grant, read, and revoke.
- Account deletion acceptance and re-authentication.
- Email verification and password-reset status codes.
- Session inventory, current-session marking, and revocation.
- Per-entry analysis acceptance, job status, conflict, and deletion.
- Privacy preference validation and read/update.
- Unified search type filters, limits, and validation.
- Export attachment and secret exclusion.

### AI Adapter Tests

Use mocked provider responses.

Test:

- Valid response.
- Invalid response.
- Timeout.
- Rate limit.
- Authentication failure.
- Malformed structured output.
- Retry behavior.

Do not make ordinary unit tests depend on live AI providers.

## 3. Security Testing

Critical tests:

- User A cannot read User B's journal.
- User A cannot update User B's conversation.
- User A cannot access User B's memories.
- User A cannot access User B's insights.
- Invalid tokens are rejected.
- Expired tokens are rejected.
- Revoked sessions are rejected.
- Invalid ownership references are rejected.
- AI retrieval never returns unauthorized data.
- Draft journals are excluded from retrieval and insights.
- Sensitive memories are excluded from retrieval without active consent.
- Sensitive memories are excluded after consent revocation.
- Refresh-token reuse revokes the token family.
- A deleted account cannot authenticate.
- Duplicate idempotency keys do not create duplicate side effects.

Authorization tests are mandatory.

## 4. Contract Testing

OpenAPI is the canonical API contract.

Validate:

- Syntax.
- Schema references.
- Required fields.
- Request and response consistency.
- Authentication definitions.
- Error response definitions.

Mobile and backend implementations must be compatible with the same contract.

## 5. Local Quality Checks

Run:

```bash
gofmt -w .
go vet ./...
go test ./...
go build ./...
go test -tags=integration ./...
npx --yes @redocly/cli@2.78.0 lint docs/shared/openapi.yaml
```

Integration tests require `TEST_DATABASE_URL` and a PostgreSQL 17 + pgvector
instance. OpenAPI lint is valid with four pre-existing warnings on the health
endpoints.

CI should also verify formatting without changing repository files.

Additional checks may include:

- Static analysis.
- Dependency vulnerability scanning.
- Container image scanning.
- OpenAPI validation.

## 6. Docker Strategy

Use a multi-stage Docker build.

Example:

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build -o /out/sela-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /out/sela-api /sela-api

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/sela-api"]
```

The Go version and runtime image must be verified against the project's actual dependencies and deployment requirements before adoption.

Requirements:

- Multi-stage build.
- Small runtime image.
- Non-root execution.
- No embedded credentials.
- Explicit port configuration.
- Graceful shutdown.
- Health endpoints.

## 7. Container Registry

Use Google Artifact Registry.

Recommended image naming:

```text
REGION-docker.pkg.dev/PROJECT_ID/REPOSITORY/sela-server:VERSION
```

Use immutable version tags for release tracking.

Avoid relying exclusively on `latest`.

## 8. Cloud Run Deployment

Configure:

- Service name.
- Region.
- Container image.
- Port.
- CPU and memory.
- Concurrency.
- Minimum and maximum instances.
- Request timeout.
- Environment variables.
- Secret references.
- Service account.
- Ingress and invocation policy.

Target region:

`asia-southeast2`

Verify availability and compatibility before deployment.

## 9. Stateless Runtime

Cloud Run instances may start, stop, and scale independently.

Therefore:

- Do not store session state in process memory.
- Do not rely on in-memory scheduled tasks.
- Store durable state in PostgreSQL.
- Use external storage for persistent files.
- Support graceful shutdown.

## 10. Database Migration Release

Recommended sequence:

1. Build and test the application.
2. Review migration files.
3. Apply compatible database migrations.
4. Deploy the new application version.
5. Run smoke tests.
6. Monitor errors and database health.

Use expand-and-contract migrations when deployed mobile clients may continue using older API versions.

Do not automatically perform destructive migrations at application startup.

## 11. CI/CD with GitHub Actions

Suggested pipeline:

```text
Push / Pull Request
       |
       v
Checkout
       |
       v
Setup Go
       |
       v
Download Dependencies
       |
       v
Format Check
       |
       v
Static Analysis
       |
       v
Unit Tests
       |
       v
Integration Tests
       |
       v
Build Binary
       |
       v
Build Docker Image
       |
       v
Security Scan
       |
       v
Push to Artifact Registry
       |
       v
Deploy Staging
       |
       v
Smoke Test
       |
       v
Production Approval
       |
       v
Deploy Production
```

Use GitHub Actions secrets or workload identity federation where appropriate.

Prefer short-lived credentials over long-lived service account keys.

## 12. Environment Separation

### Development

- Local configuration.
- Development database.
- Test provider credentials.

### Staging

- Production-like configuration.
- Isolated database.
- Controlled test accounts.
- Integration and smoke tests.

### Production

- Restricted secrets.
- Production database.
- Monitoring.
- Alerts.
- Controlled releases.

Never use production private journals as ordinary test fixtures.

## 13. Health Checks

Implement:

`GET /api/v1/health/live`

Purpose:

- Confirm that the process is alive.

`GET /api/v1/health/ready`

Purpose:

- Confirm that the application is ready to serve traffic.

Readiness may check critical dependencies with bounded timeouts.

Do not expose credentials or internal infrastructure details.

## 14. Observability

Monitor:

- Request volume.
- Error rates.
- Latency.
- Database connections.
- Slow queries.
- AI provider latency.
- Token usage.
- Provider failures.
- Job backlog.
- Job failures.
- Deployment revisions.

Set alerts for significant operational failures.

## 15. Rollback and Recovery

Maintain a documented release recovery strategy.

For application deployment:

- Retain known-good revisions.
- Validate the new revision.
- Shift traffic carefully.
- Roll back when necessary.

For database changes:

- Prefer backward-compatible migrations.
- Use roll-forward repair for irreversible schema changes.
- Restore from backup only after understanding data-loss implications.

Test recovery procedures periodically.

## 16. Release Checklist

### Code

- [x] Formatting verified.
- [x] Static analysis passes.
- [x] Unit tests pass.
- [x] Integration tests pass.
- [x] Authorization tests pass.
- [x] Async insight job tests pass.
- [x] Scheduled insight preference, idempotency, and opt-out tests pass.
- [x] Sensitive memory consent and revocation tests pass.
- [x] Prompt v2 deterministic behavior gates pass.
- [x] Idempotency tests pass.
- [x] Account deletion tests pass.
- [x] OpenAPI validated.
- [x] Email verification and password reset tests pass.
- [x] Session inventory and revocation tests pass.
- [x] Per-entry journal analysis tests pass.
- [x] Privacy preference and enforcement tests pass.
- [x] Unified search tests pass.

### Database

- [x] Migrations reviewed through `21`.
- [x] Indexes reviewed, including search GIN indexes.
- [x] Backup strategy confirmed locally.
- [x] Connection limits reviewed.

### AI

- [x] Provider credentials configured for local staging.
- [x] Timeouts configured.
- [x] Usage tracking enabled.
- [x] Token limits configured.
- [x] Privacy behavior reviewed for Phase 4B.

### Infrastructure

- [x] Docker image built for local staging.
- [ ] Security scan completed.
- [ ] Image pushed to Artifact Registry (GCP deferred).
- [ ] Cloud Run configuration reviewed (GCP deferred).
- [x] Local secrets configured outside source control.
- [x] Local health checks pass.
- [ ] Hosted monitoring and alerts active (GCP deferred).

### Product and Privacy

- [x] Journal ownership verified.
- [x] Memory provenance verified.
- [x] Insight evidence verified.
- [x] Schedule lifecycle and weekly scheduled generation verified locally;
      monthly period computation is covered by unit tests.
- [x] Sensitive memory consent verified, including revocation.
- [x] Prompt v2 deterministic behavior verified.
- [ ] Live model quality evaluation completed with the production model.
- [x] Local Docker scheduler verified; hosted scheduler trigger pending.
- [x] Sensitive logs reviewed.
- [x] Data deletion behavior tested locally.
- [x] Email verification, reset, session inventory, and export verified.
- [x] Journal analysis verified locally with a real provider.
- [x] Privacy enforcement and unified search verified locally.

## 17. Production Acceptance

The backend is ready for production when:

- Critical tests pass.
- Authorization is verified.
- OpenAPI matches the deployed API.
- Database migrations are controlled.
- Secrets are managed securely.
- AI requests have bounded timeouts and costs.
- Background jobs are durable.
- Monitoring and alerting are active.
- Backup and recovery are documented.
- Release recovery procedures are available.
