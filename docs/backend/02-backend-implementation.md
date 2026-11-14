# SELA Backend Implementation Guide

Version: 1.1  
Status: MVP baseline

## 1. Purpose

This document translates the SELA architecture into an implementation plan for the Go backend.

The backend must provide reliable APIs while preserving clear boundaries between business logic, persistence, and AI integration.

## 2. Application Bootstrap

The application startup process should:

1. Load configuration.
2. Initialize structured logging.
3. Establish database connections.
4. Initialize repositories.
5. Initialize service dependencies.
6. Initialize external provider adapters.
7. Register middleware.
8. Register routes.
9. Start the HTTP server.
10. Handle graceful shutdown.

Avoid initializing external dependencies inside individual HTTP handlers.

## 3. HTTP Layer

Use Gin for routing and middleware.

Recommended middleware:

- Request ID.
- Recovery.
- Structured access logging.
- CORS.
- Authentication.
- Rate limiting.
- Request size limits.
- Timeout handling where appropriate.

Public routes and protected routes must be separated.

Example:

```go
router := gin.New()

router.Use(
    gin.Recovery(),
    RequestIDMiddleware(),
    LoggingMiddleware(),
)

api := router.Group("/api/v1")

api.GET("/health/live", healthHandler.Live)
api.GET("/health/ready", healthHandler.Ready)

auth := api.Group("/auth")
auth.POST("/register", authHandler.Register)
auth.POST("/login", authHandler.Login)
auth.POST("/refresh", authHandler.Refresh)

protected := api.Group("")
protected.Use(AuthenticationMiddleware())

protected.GET("/me", userHandler.GetMe)
protected.POST("/journals", journalHandler.Create)
```

This is illustrative routing code. Production middleware must be fully implemented and tested.

## 4. Authentication Module

Responsibilities:

- Registration.
- Login.
- Password verification.
- Access token creation.
- Refresh token management.
- Logout.
- Session revocation.

Requirements:

- Normalize email addresses.
- Hash passwords with a modern password-hashing algorithm.
- Store refresh-token hashes.
- Validate token expiration.
- Revoke sessions when required.
- Rate-limit authentication endpoints.

The authenticated user identity must be derived from verified credentials.

## 5. User Module

Responsibilities:

- Profile retrieval.
- Profile updates.
- Preference management.
- Communication style preferences.
- Timezone and locale settings.

Avoid collecting profile attributes that are not required by the product.

Profile data and user preferences should be independently manageable.

## 6. Journal Module

Responsibilities:

- Create journal.
- List journals.
- Retrieve journal detail.
- Update journal.
- Delete journal.
- Search journals.

Implementation rules:

1. Validate the request.
2. Obtain user identity from authentication context.
3. Persist the original journal.
4. Return a stable response DTO.
5. Queue optional processing when required.

Concurrency rules:

- Every journal carries a `version`.
- Updates and deletes require `If-Match`.
- Increment `version` with a conditional update inside a transaction.
- Return `409 VERSION_CONFLICT` when the version does not match.
- Journal creation requires an `Idempotency-Key`.

Do not modify original journal text during AI analysis.

AI-generated analysis must be stored separately.

### Journal Processing

Only journals with status `active` are eligible for AI processing.

A journal may trigger:

- Embedding generation.
- Memory extraction.
- Optional summarization.

Processing should be asynchronous when it is not necessary to complete the user's save operation.

A journal save should not fail solely because an external AI provider is unavailable.

## 7. Conversation Module

Responsibilities:

- Create conversations.
- List conversations.
- Retrieve conversation history.
- Delete conversations.
- Store messages.
- Coordinate Companion requests.

### Message Flow

1. Validate conversation ID.
2. Verify conversation ownership.
3. Validate message content.
4. Persist the user message.
5. Invoke AI orchestration.
6. Persist the assistant response.
7. Return the result.

If generation fails, the system must preserve enough state to represent the failed or retryable operation.

Do not silently duplicate user messages during retries.

## 8. AI Module Boundary

Business modules must not call provider SDKs directly.

The AI module should expose application-level interfaces for:

- Text generation.
- Embedding generation.
- Context retrieval.
- Memory extraction.
- Insight generation.

Provider-specific implementations belong behind adapters.

## 9. Memory Module

Responsibilities:

- Store memory candidates.
- Validate source references.
- Manage memory status.
- Support confirmation and rejection.
- Retrieve eligible memories.
- Handle conflicts and superseded records.

Memory extraction should be a separate operation from journal persistence.

A memory must retain source provenance.

Users must be able to inspect and correct their personal memories.

## 10. Insight Module

Responsibilities:

- Generate on-demand insights.
- Store insight results.
- Link evidence.
- Provide insight history.
- Accept user feedback.

Insight generation should:

- Gather relevant eligible records.
- Avoid using unrelated context.
- Require evidence references.
- Distinguish observations from interpretations.
- Return a clear result when evidence is insufficient.

## 11. Repository and Database Layer

Repositories should encapsulate persistence.

Examples:

- UserRepository.
- JournalRepository.
- ConversationRepository.
- MessageRepository.
- MemoryRepository.
- InsightRepository.
- JobRepository.

Use transactions when an operation requires multiple related records to be committed atomically.

For AI operations, avoid holding database transactions open while waiting for a slow external provider.

## 12. Database Connection Management

Configure:

- Maximum open connections.
- Maximum idle connections.
- Connection maximum lifetime.
- Connection idle lifetime.
- Query timeout.

Consider Cloud Run instance scaling when determining total possible database connections.

Use pooled Neon connections for compatible application workloads and direct connections for migration or administrative tasks when required.

## 13. Background Job Implementation

Initial job types:

| Job | Purpose |
|---|---|
| `journal.embedding` | Generate journal embeddings |
| `journal.memory_extraction` | Extract candidate memories |
| `message.memory_extraction` | Extract candidate memories from conversations |
| `insight.generate` | Generate insights |
| `account.delete` | Purge user-owned data after account deletion |
| `data.cleanup` | Perform eligible cleanup |

Job lifecycle:

```text
pending
   |
   v
processing
   |
   +---- completed
   |
   +---- retry
   |
   +---- failed
```

A worker must:

- Claim jobs safely.
- Use a lock or equivalent concurrency control.
- Track attempts.
- Support retries.
- Record failure details.
- Avoid duplicate side effects.
- Release or expire abandoned locks.

Use durable PostgreSQL-backed jobs initially.

Do not introduce a message broker until operational requirements justify it.

## 14. API Documentation

Maintain the OpenAPI contract in:

`docs/shared/openapi.yaml`

The contract must reflect actual implementation.

When an endpoint changes:

1. Update OpenAPI.
2. Validate the specification.
3. Update backend tests.
4. Notify the mobile implementation.
5. Review compatibility.

Expose Swagger UI or Redoc in development and staging. Production access should be controlled according to operational policy.

## 15. Observability

Each request should have a request ID.

Record:

- HTTP method.
- Route.
- Status code.
- Latency.
- User identifier where appropriate.
- Error category.

For AI operations, record:

- Provider.
- Model.
- Input tokens.
- Output tokens.
- Duration.
- Error category.

Never log full journal or message content by default.

## 16. Implementation Order

### Phase 1: Foundation

- Application bootstrap.
- Configuration.
- Logging.
- Database.
- Migrations.
- Health endpoints.

### Phase 2: Core User Features

- Authentication.
- Profile.
- Preferences.
- Journal CRUD.
- Conversation persistence.

### Phase 3: AI Capabilities

- Provider abstraction.
- Companion generation.
- Context retrieval.
- Embedding generation.
- Memory extraction.

### Phase 4: Insights

- Insight generation.
- Evidence persistence.
- User feedback.
- Scheduled processing.

### Phase 5: Production

- Security review.
- Integration tests.
- Docker.
- CI/CD.
- Cloud Run.
- Monitoring.
- Backup and recovery.

## 17. Implementation Acceptance

The backend implementation is acceptable when:

- Module boundaries are clear.
- API ownership checks are enforced.
- Journal persistence is independent of AI availability.
- Conversations are persisted reliably.
- Memory records retain provenance.
- Insights have evidence references.
- Background jobs are durable.
- Database migrations are versioned.
- OpenAPI matches the implementation.
- Critical flows have automated tests.
