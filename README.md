# SELA

**A space to understand yourself.**

SELA is an AI-powered journaling and personal reflection companion designed to help people understand their experiences, recognize recurring patterns, connect present situations with past experiences, and make more intentional decisions.

Our philosophy:

> Remember. Understand. Grow.

## Product Principles

1. Personal-first: the user's own journal is the primary source of personal context.
2. Reflection over judgment: SELA helps users understand, not shame themselves.
3. Evidence-aware: insights must be grounded in available context and distinguish facts from hypotheses.
4. User agency: users can inspect, correct, reject, and delete personal memories.
5. Simple experience, intelligent system: the interface stays focused while backend intelligence handles complexity.
6. Privacy by default: personal journal content must be protected throughout its lifecycle.

## Backend Status

Backend Phase 2 is complete and frozen at `v0.2.0`.

- API contract: OpenAPI `1.3.0`.
- Database schema: migration version `16`.
- Release notes: `CHANGELOG.md`.
- Session handoff: `HANDOFF.md`.
- Freeze checklist and next inputs: `docs/backend/10-phase-2-freeze.md`.
- Active focus phase: `docs/backend/11-focus-phase.md`.

The mobile application, staging deployment, real embedding provider, and Phase 3
beta hardening are separate deliverables.

## Repositories

SELA uses two repositories.

### Backend

Repository: `sela-server`

Responsibilities:

- REST API.
- Authentication and authorization.
- Journal and conversation management.
- AI orchestration.
- Memory extraction and retrieval.
- Insight generation.
- Background jobs.
- Database management.
- Deployment and observability.

### Mobile

Repository: `sela-mobile`

Responsibilities:

- Flutter application.
- User onboarding.
- Journal writing experience.
- AI Companion interface.
- Insight presentation.
- Profile and privacy controls.
- Mobile session management.

## Documentation Structure

The backend repository is the canonical source for shared product and technical documentation.

```text
sela-server/
├── docs/
│   ├── shared/
│   │   ├── 01-product-specification.md
│   │   ├── 02-product-frameworks.md
│   │   ├── 03-system-architecture.md
│   │   ├── 04-database-design.md
│   │   ├── 05-ai-system-design.md
│   │   ├── 06-api-contract.md
│   │   ├── 07-security-and-privacy.md
│   │   ├── 08-operations-and-reliability.md
│   │   ├── 09-roadmap.md
│   │   ├── openapi.yaml
│   │   └── database-erd.dbml
│   └── backend/
│       ├── 01-development-guide.md
│       ├── 02-backend-implementation.md
│       ├── 03-ai-integration.md
│       └── 04-testing-and-deployment.md
└── README.md

sela-mobile/
├── docs/
│   └── flutter/
│       ├── 01-development-guide.md
│       ├── 02-ui-ux-specification.md
│       ├── 03-mobile-implementation.md
│       └── 04-testing-and-release.md
└── README.md
```

## Technology Baseline

| Layer | Technology |
|---|---|
| Mobile | Flutter and Dart |
| Backend | Go and Gin |
| Architecture | Modular monolith |
| ORM | GORM |
| Database | Neon PostgreSQL |
| Vector search | pgvector |
| API specification | OpenAPI 3.1 |
| API documentation | Swagger UI or Redoc |
| Container | Docker |
| Container registry | Google Artifact Registry |
| Deployment | Google Cloud Run |
| CI/CD | GitHub Actions |
| Scheduled execution | Cloud Scheduler and Cloud Run Jobs |
| Cache | Provider prompt caching initially |
| Object storage | Cloudflare R2, optional |

## Important Architectural Decisions

- Start with a modular monolith, not microservices.
- PostgreSQL is the source of truth.
- Use SQL migrations. Do not use GORM AutoMigrate in production.
- Use hybrid retrieval: PostgreSQL full-text search and pgvector.
- Keep embedding data derived from original records.
- Abstract LLM and embedding providers behind independent interfaces.
- Do not introduce Redis until actual requirements justify it.
- Do not introduce a separate Python inference service in the initial architecture.
- Use durable database-backed jobs for asynchronous processing.
- Cloud Run services must remain stateless.
- Do not use in-memory timers for scheduled production tasks.
- Use Android-first development when working on Windows. iOS builds require macOS and Xcode.

## Documentation Rules

The words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY express requirement strength.

- MUST: mandatory requirement.
- SHOULD: recommended unless a documented reason exists.
- MAY: optional implementation.

If implementation conflicts with this documentation, document the reason and update the relevant decision.

## Recommended Reading Order

1. Product specification.
2. Product frameworks.
3. System architecture.
4. Database design.
5. AI system design.
6. API contract.
7. Security and privacy.
8. Operations and reliability.
9. Roadmap.
10. Backend or Flutter implementation documents.

## Change Management

Any change affecting the following must be reviewed across both repositories:

- API contracts.
- Database schema.
- Authentication.
- Journal ownership and privacy.
- Memory extraction and retrieval.
- AI response behavior.
- User-facing product modules.

The backend repository owns the canonical shared documentation. The mobile repository should reference the shared contract rather than independently redefining it.
