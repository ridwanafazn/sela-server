# SELA Operations and Reliability

Version: 1.1

## 1. Objective

This document defines operational practices for deploying and maintaining SELA.

The initial infrastructure should remain simple and cost-conscious while protecting user data.

## 2. Environments

Maintain separate environments:

### Development

- Local backend.
- Local Flutter application.
- Development database.
- Test AI credentials.

### Staging

- Production-like deployment.
- Separate database.
- Controlled test accounts.
- Integration testing.

### Production

- Restricted access.
- Production database.
- Managed secrets.
- Monitoring and alerting.
- Controlled releases.

Never use production journal data in development by default.

## 3. Infrastructure

Initial infrastructure:

| Component | Service |
|---|---|
| API | Google Cloud Run |
| Container registry | Google Artifact Registry |
| Database | Neon PostgreSQL |
| Vector search | pgvector |
| Scheduled trigger | Cloud Scheduler |
| Background execution | Cloud Run Jobs or worker deployment |
| CI/CD | GitHub Actions |
| Optional object storage | Cloudflare R2 |

Target Cloud Run region: `asia-southeast2`, subject to current availability and service compatibility.

## 4. Container Strategy

Backend must be containerized.

Recommended properties:

- Multi-stage build.
- Small runtime image.
- Non-root execution.
- Explicit port configuration.
- Health endpoints.
- Environment-based configuration.
- No embedded secrets.
- Graceful shutdown.

## 5. CI/CD

Suggested pipeline:

1. Checkout.
2. Install dependencies.
3. Format check.
4. Static analysis.
5. Unit tests.
6. Integration tests where available.
7. Build binary.
8. Build Docker image.
9. Security scan.
10. Push image to Artifact Registry.
11. Deploy to staging.
12. Smoke test.
13. Controlled production deployment.

Production deployment should require appropriate approval or release conditions.

## 6. Database Release

Database migrations are a controlled release step.

Requirements:

- Versioned migration files.
- Review before production.
- Backup or recovery strategy.
- Safe migration order.
- Compatibility with deployed application versions.
- Roll-forward plan for irreversible changes.

Avoid relying on application startup to perform production schema changes.

## 7. Background Jobs

Jobs must be durable.

A worker should:

- Claim eligible jobs safely.
- Lock jobs with expiration.
- Track attempts.
- Record failures.
- Retry transient errors.
- Stop after a maximum attempt count.
- Avoid duplicate side effects.

Use idempotency keys where appropriate.

## 8. Scheduling

Use Cloud Scheduler for periodic triggers.

Use Cloud Run Jobs or a protected worker mechanism for execution.

Do not use in-memory timers in Cloud Run services.

Scheduled insight generation is post-MVP. The durable job infrastructure should be prepared for it, but no schedule preference API is included in the MVP.

When introduced, scheduled insight generation should be:

- User-configurable.
- Timezone-aware.
- Idempotent.
- Cost-controlled.
- Easy to disable.

## 9. Observability

Monitor:

### API

- Request count.
- Error rate.
- Latency.
- Status code distribution.
- Authentication failures.

### Database

- Connection usage.
- Query latency.
- Connection exhaustion.
- Slow queries.
- Migration failures.

### AI

- Provider availability.
- Model latency.
- Token usage.
- Cost.
- Timeout rate.
- Error rate.
- Output validation failures.

### Jobs

- Queue depth.
- Oldest pending job.
- Failure count.
- Retry count.
- Processing duration.

## 10. Logging

Use structured logs.

Include:

- Timestamp.
- Severity.
- Request ID.
- Component.
- Operation.
- Error category.

Never log complete journal content by default.

## 11. Reliability Targets

Initial engineering targets, not contractual SLAs:

- API availability target: 99.5% monthly after production stabilization.
- Ordinary API p95 latency: below 500 ms.
- AI response latency: tracked separately.
- Background job failures: visible and recoverable.
- Database recovery: documented and periodically tested.

## 12. Backup and Recovery

Define:

- Backup schedule.
- Retention.
- Recovery point objective.
- Recovery time objective.
- Restore procedure.
- Responsibility.

Verify Neon backup and restore capabilities and limitations before finalizing production recovery commitments.

A backup is not sufficient until restoration has been tested.

## 13. Cost Management

Track:

- Cloud Run usage.
- Database usage.
- AI input and output tokens.
- Embedding generation.
- Storage.
- Logs.
- Background processing.

Cost controls:

- Token limits.
- Context limits.
- Rate limits.
- Bounded retries.
- On-demand generation.
- Scheduled job limits.
- Prompt caching where appropriate.

Do not generate expensive insights without a clear user benefit.

## 14. Operational Runbooks

Maintain procedures for:

- API unavailable.
- Database unavailable.
- AI provider unavailable.
- Job queue backlog.
- Token or cost spike.
- Failed deployment.
- Failed migration.
- Suspected data exposure.

## 15. Production Readiness Checklist

- [ ] Environment separation.
- [ ] Secrets configured.
- [ ] Authentication verified.
- [ ] Ownership authorization verified.
- [ ] Database migrations reviewed.
- [ ] Backup and restore tested.
- [ ] AI provider timeout configured.
- [ ] Rate limits configured.
- [ ] Sensitive logging reviewed.
- [ ] Health endpoints available.
- [ ] Monitoring configured.
- [ ] Alerts configured.
- [ ] CI/CD verified.
- [ ] Rollback or roll-forward plan documented.
- [ ] Data deletion process tested.
- [ ] Incident response documented.
