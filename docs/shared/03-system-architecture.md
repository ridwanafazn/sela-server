# SELA System Architecture

Version: 1.2

> Scope note (2026-11-21): weekly and monthly scheduled insights are MVP.
> Per-entry journal analysis is a core product capability with post-MVP
> delivery; its backend is implemented in Phase 4A. Sensitive memory requires
> explicit consent before storage and retrieval. See
> `10-product-vision-decisions.md`.

## 1. Architecture Overview

SELA uses a modular monolith architecture.

The initial system consists of:

- Flutter mobile application.
- Go backend using Gin.
- Neon PostgreSQL.
- pgvector.
- External LLM provider.
- External embedding provider.
- Google Cloud Run.
- Google Artifact Registry.
- GitHub Actions.

The architecture prioritizes operational simplicity and clear module boundaries.

Microservices are not required for the MVP.

## 2. High-level Architecture

```text
Flutter Mobile
     |
     | HTTPS / REST / JWT
     v
Go API (Gin)
     |
     +----------------------+
     |                      |
     v                      v
Application Modules     AI Orchestrator
     |                      |
     |                      +---- LLM Provider
     |                      |
     |                      +---- Embedding Provider
     |                      |
     v                      v
Repositories          Context Retrieval
     |                      |
     +----------+-----------+
                |
                v
         Neon PostgreSQL
          + pgvector

Background Jobs
     |
     +---- Memory Extraction
     +---- Embedding Generation
     +---- Scheduled Insights
     +---- Cleanup and Maintenance
```

## 3. Technology Decisions

| Component | Decision |
|---|---|
| Mobile | Flutter |
| Backend | Go |
| HTTP framework | Gin |
| ORM | GORM |
| Database | Neon PostgreSQL |
| Vector extension | pgvector |
| Deployment | Cloud Run |
| Registry | Artifact Registry |
| CI/CD | GitHub Actions |
| API contract | OpenAPI 3.1 |
| Containerization | Docker |

## 4. Backend Modules

The backend should be organized by business capability.

### Authentication

Responsibilities:

- Registration.
- Login.
- Token refresh.
- Logout.
- Session management.

### User

Responsibilities:

- Profile.
- Preferences.
- Communication style.
- User settings.

### Journal

Responsibilities:

- Journal CRUD.
- Journal history.
- Journal search.
- Journal ownership.

### Conversation

Responsibilities:

- Conversation management.
- Message persistence.
- Message history.
- AI response orchestration.

### Memory

Responsibilities:

- Memory extraction.
- Memory persistence.
- Memory provenance.
- Memory feedback.
- Memory retrieval.

### Insight

Responsibilities:

- On-demand insight generation.
- Scheduled weekly and monthly insight generation.
- User-configurable schedule preferences.
- Evidence references.
- Insight feedback.

### AI

Responsibilities:

- Provider adapters.
- Prompt construction.
- Context assembly.
- Retrieval.
- Token usage.
- Model configuration.

### Job

Responsibilities:

- Durable asynchronous tasks.
- Retries.
- Job locking.
- Scheduled execution.

## 5. Data Ownership

PostgreSQL is the authoritative source of application data.

Original journals and conversations must remain the source material.

Derived records include:

- Embeddings.
- Structured memories.
- Insights.
- Summaries.

Derived records must retain references to their sources whenever applicable.

Embeddings are disposable derived data and can be regenerated from eligible source records.

## 6. Main Data Flows

### Journal Creation

1. Mobile sends journal request.
2. API authenticates user.
3. API validates request.
4. Journal module stores the original content.
5. API returns the stored journal.
6. Optional asynchronous jobs are queued.

### Companion Conversation

1. Mobile sends message.
2. API validates conversation ownership.
3. Message is persisted.
4. Context retrieval finds relevant records.
5. Context builder applies privacy and relevance rules.
6. AI provider generates response.
7. Response is persisted.
8. API returns the response.

### Memory Extraction

1. A journal or conversation becomes eligible for processing.
2. A durable job is created.
3. Worker loads the authorized source.
4. AI extracts candidate memories.
5. Candidates are validated.
6. Memories are stored with provenance and confidence.
7. User can review or correct them.

### Insight Generation

1. User requests an insight or a scheduled job is triggered.
2. Backend gathers eligible source records.
3. Retrieval and aggregation select relevant context.
4. AI generates candidate observations.
5. Evidence references are validated.
6. Insight is persisted.
7. User reviews the result.

### Scheduled Insight Generation

1. User enables a weekly or monthly schedule.
2. A scheduler enqueues durable jobs for eligible users and periods.
3. The worker applies the user's timezone, ownership, eligibility, evidence
   threshold, and AI budget.
4. The insight is persisted with evidence, or the job completes with
   `insufficient_evidence`.
5. The user can disable the schedule at any time.

Per-entry journal analysis is post-MVP and its backend is implemented. A
request creates a durable `journal.analysis` job. The worker loads only the
owned entry, checks its version and publication state, calls the model, stores
the result separately, and returns an explicit outcome. The MVP reflection path
remains Companion and period-based insights.

## 7. Retrieval Architecture

SELA uses hybrid retrieval.

Components:

- PostgreSQL relational filtering.
- PostgreSQL full-text search.
- pgvector similarity search.
- Relevance ranking.
- Context assembly.

The database remains the source of truth.

Vector search must never bypass user ownership or privacy filters.

## 8. Background Processing

Background tasks may include:

- Embedding generation.
- Memory extraction.
- Insight generation.
- Scheduled summaries.
- Data cleanup.

Use durable database-backed jobs initially.

A job should include:

- Job ID.
- Job type.
- Payload.
- Status.
- Attempt count.
- Next execution time.
- Lock information.
- Error information.
- Created and updated timestamps.

Jobs must be idempotent where possible.

## 9. Scheduled Execution

Cloud Run services are stateless.

Do not use an in-memory ticker as the production scheduling mechanism.

Recommended approach:

- Cloud Scheduler triggers a protected endpoint or Cloud Run Job.
- The backend creates or claims durable jobs.
- Workers process eligible jobs.
- Failed jobs are retried according to policy.

Scheduled insight generation is MVP. It must be user-configurable,
timezone-aware, idempotent, cost-controlled, opt-in, and easy to disable.

## 10. Deployment Architecture

Initial deployment:

- Backend container: Google Cloud Run.
- Container registry: Google Artifact Registry.
- Database: Neon PostgreSQL.
- Scheduled tasks: Cloud Scheduler and Cloud Run Jobs.
- CI/CD: GitHub Actions.

Target region: asia-southeast2.

Verify current service availability, latency, provider support, and data residency requirements before production deployment.

## 11. Connection Management

Use Neon pooled connections for compatible application traffic.

Use direct connections for migrations or administrative operations when required.

Configure:

- Maximum open connections.
- Maximum idle connections.
- Connection lifetime.
- Request timeout.
- Database timeout.

Validate the current Neon connection guidance before production.

## 12. AI Provider Abstraction

LLM and embedding providers must be independent interfaces.

Avoid coupling business logic to a specific provider's SDK.

Provider adapters should handle:

- Authentication.
- Request formatting.
- Response parsing.
- Timeouts.
- Retries.
- Usage metadata.
- Provider-specific errors.

## 13. Security Boundaries

Every user-owned resource must be authorized at the application layer.

Never trust user IDs supplied in request bodies for ownership decisions.

Use the authenticated identity as the authority.

AI retrieval must only access records belonging to the authenticated user and eligible for the current operation.

Sensitive memories may be stored only with explicit user consent. They must be
excluded from retrieval when consent is absent, expired, or revoked.

## 14. Architecture Constraints

Do not introduce the following without evidence of need:

- Microservices.
- Redis.
- Kafka.
- Kubernetes.
- Dedicated Python inference services.
- Multi-agent orchestration.
- Complex event streaming.

Prefer the smallest architecture that safely supports the product.
