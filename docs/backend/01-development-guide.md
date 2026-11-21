# SELA Backend Development Guide

Version: 1.1

Status: current backend through Phase 4B

## 1. Purpose

This document defines the development standards, tooling, project structure, and workflow for the SELA backend.

The backend is responsible for authentication, user data, journals, conversations, AI orchestration, memory, insights, and background processing.

## 2. Technology Stack

| Layer | Technology |
|---|---|
| Language | Go |
| HTTP framework | Gin |
| ORM | GORM |
| Database | Neon PostgreSQL |
| Vector extension | pgvector |
| API specification | OpenAPI 3.1 |
| API documentation | Swagger UI or Redoc |
| Containerization | Docker |
| Deployment | Local Docker staging (verified); Google Cloud Run (deferred) |
| Registry | Local image; Google Artifact Registry (deferred) |
| CI/CD | GitHub Actions |
| Migration | `golang-migrate` embedded migrations |

Use stable, compatible versions and commit dependency lock information where applicable.

## 3. Architecture

SELA uses a modular monolith.

Each module owns a specific business capability. Modules should communicate through defined interfaces rather than accessing each other's internal implementation directly.

Current modules:

- Account
- AI
- Auth
- Conversation
- Insight
- Job
- Journal
- Journal analysis
- Memory
- Pre-public account gates
- Privacy
- Search
- User

Platform packages cover AI usage and budgets, database, health, HTTP helpers,
logging, mail, rate limiting, and safety.

Do not introduce microservices for the MVP.

## 4. Project Structure

The tree below is an early baseline and may not match every current package.
The module list above and `internal/modules` are authoritative. Modules remain
flat packages; handlers, services, repositories, and models live together.

```text
sela-server/
|-- cmd/{api,migrate,ops,scheduler,worker}
|-- internal/app
|-- internal/config
|-- internal/evaluation
|-- internal/modules/{account,ai,auth,conversation,insight,job,journal,journalanalysis,memory,prepublic,privacy,search,user}
|-- internal/platform/{aiusage,database,health,httpx,logger,mailer,ratelimit,safety}
|-- internal/shared/{apierrors,authn,clientip,ids,optional,requestid,response,textquery}
|-- migrations
|-- prompts
|-- docs/{shared,backend}
|-- scripts
|-- test/integration
|-- Dockerfile
|-- compose.staging.yml
|-- docker-compose.yml
|-- Makefile
|-- go.mod
|-- .env.example
|-- CHANGELOG.md
|-- HANDOFF.md
`-- README.md
```

## 5. Layer Responsibilities

### Handler

Responsible for:

- HTTP request parsing.
- Input validation.
- Calling application services.
- HTTP status codes.
- Response serialization.

Handlers must not contain substantial business logic.

### Service

Responsible for:

- Business rules.
- Authorization decisions within the application flow.
- Coordinating repositories.
- Transaction boundaries.
- Calling external interfaces.

### Repository

Responsible for:

- Database queries.
- Persistence.
- Data retrieval.
- Database-specific operations.

### Model

Contains:

- Domain entities.
- Persistence models.
- Internal data structures.

Avoid exposing database models directly as public API responses.

## 6. Environment Configuration

Configuration must be provided through environment variables or a managed configuration mechanism.

Example `.env.example`:

```env
APP_ENV=development
APP_PORT=8080
APP_NAME=sela-server

DATABASE_URL=
DATABASE_MAX_OPEN_CONNS=10
DATABASE_MAX_IDLE_CONNS=5

JWT_ACCESS_SECRET=
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h

LLM_PROVIDER=
LLM_API_KEY=
LLM_MODEL=

EMBEDDING_PROVIDER=
EMBEDDING_API_KEY=
EMBEDDING_MODEL=

LOG_LEVEL=debug
```

These values are illustrative defaults, not production secrets or final capacity settings.

Rules:

- Never commit real credentials.
- Validate required configuration during startup.
- Fail fast when critical configuration is missing.
- Separate development, staging, and production credentials.

## 7. Local Development Setup

Prerequisites:

- Go.
- Git.
- Docker.
- PostgreSQL-compatible database access.
- API testing tool.

Flutter SDK is not required for backend development.

Suggested workflow:

1. Clone the repository.
2. Install Go dependencies.
3. Configure environment variables.
4. Start local dependencies if needed.
5. Run database migrations.
6. Start the API.
7. Test health endpoints.
8. Run automated tests.

Example commands:

```bash
go mod download
go mod tidy
go run ./cmd/api
```

Migration commands depend on the selected migration tool and should be documented in the project Makefile.

## 8. Coding Standards

### General

- Follow standard Go formatting.
- Use meaningful names.
- Keep functions focused.
- Avoid unnecessary abstraction.
- Return errors explicitly.
- Propagate context cancellation.
- Avoid global mutable state.

### Error Handling

Errors should be classified into application-level categories.

Examples:

- Validation error.
- Unauthorized.
- Forbidden.
- Not found.
- Conflict.
- Internal error.
- Upstream provider error.

Do not return raw database or provider errors to clients.

### Context

Use `context.Context` for:

- HTTP request propagation.
- Database operations.
- External AI requests.
- Background job cancellation.

Do not store request-specific state in global variables.

### Logging

Use structured logging.

Include request identifiers and operation metadata.

Do not log complete journal content or private conversations.

## 9. Database Standards

- Use GORM for ordinary persistence operations.
- Use SQL where explicit PostgreSQL features are needed.
- Use versioned migrations.
- Do not use GORM AutoMigrate in production.
- Add appropriate foreign keys and indexes.
- Use transactions for multi-record operations.
- Avoid unbounded database queries.

## 10. API Standards

- Base path: `/api/v1`.
- JSON request and response bodies.
- Consistent error structure.
- Explicit validation.
- Cursor pagination for large collections.
- Authentication middleware for protected endpoints.
- Resource ownership checks.

The OpenAPI document is the canonical machine-readable contract.

## 11. Testing Standards

Required checks:

```bash
gofmt -w .
go vet ./...
go test ./...
go build ./...
```

Formatting checks in CI should verify formatting without silently modifying committed source files.

Add:

- Unit tests.
- Integration tests.
- Authorization tests.
- API contract tests.
- AI adapter tests.

## 12. Git Workflow

Recommended branches:

- `main`: stable branch.
- `develop`: integration branch, if the team needs it.
- `feature/*`: feature work.
- `fix/*`: bug fixes.
- `hotfix/*`: urgent production fixes.

Pull requests should include:

- Purpose.
- Scope.
- Relevant issue or requirement.
- Test results.
- Database migration impact.
- API contract impact.
- Security and privacy impact.

## 13. Development Sequence

Recommended order:

1. Project bootstrap.
2. Configuration and logging.
3. Database connection.
4. Migration setup.
5. Health endpoints.
6. Authentication.
7. User profile.
8. Journal CRUD.
9. Conversation persistence.
10. AI provider integration.
11. Context retrieval.
12. Memory extraction.
13. Insight generation.
14. Background jobs.
15. Deployment and monitoring.

## 14. Definition of Done

A backend feature is complete when:

- Business requirements are implemented.
- Authorization is enforced.
- Input validation exists.
- Error handling is consistent.
- Tests cover important behavior.
- API documentation is updated.
- Logs avoid sensitive content.
- Database migrations are included when required.
- Relevant shared documentation is updated.
