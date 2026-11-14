# SELA API Contract

Version: 1.2  
Status: P0 decisions locked

## 1. Purpose

This document defines the conventions that govern the SELA HTTP API.

It explains authentication, authorization, response envelopes, errors, pagination, idempotency, rate limits, versioning, and AI generation behavior.

It does **not** duplicate the endpoint inventory. The endpoint inventory, request schemas, response schemas, and examples live in:

`docs/shared/openapi.yaml`

## 2. Canonical Contract

`openapi.yaml` is the single machine-readable API contract.

Rules:

- If this document and `openapi.yaml` disagree, `openapi.yaml` wins.
- Backend and mobile implementations MUST be generated from, or verified against, `openapi.yaml`.
- Endpoint tables MUST NOT be maintained in this document.
- A new endpoint is not part of the contract until it exists in `openapi.yaml` and passes contract validation.
- Proposed endpoints MUST be clearly marked as proposed and MUST NOT be presented as implemented.

## 3. API Style

- REST over HTTPS.
- JSON request and response bodies.
- Versioned base path: `/api/v1`.
- Resource-oriented paths.
- Stable resource identifiers.
- Explicit pagination for collections.
- Explicit idempotency for expensive or retry-sensitive writes.
- Consistent error responses.
- No internal provider payloads, prompts, stack traces, or database errors in responses.

## 4. Authentication and Authorization

The API uses short-lived JWT access tokens and rotating refresh tokens.

### Access Token

Authenticated requests use:

```http
Authorization: Bearer <access_token>
```

The backend MUST validate:

- Signature.
- Expiration.
- Issuer.
- Audience.
- Session identifier.
- Revocation state where applicable.

### Refresh Token

Refresh tokens are opaque, stored as hashes, rotated on use, and grouped into a token family for reuse detection.

Refresh-token reuse MUST revoke the token family.

### Logout

Logout revokes the current session. Local client cleanup MUST still occur if the network request fails.

### Authorization

Every user-owned resource MUST be scoped to the authenticated user.

Ownership is enforced by the backend, never by a client-supplied user ID.

For resources not owned by the caller, the API SHOULD return `404 Not Found` rather than revealing their existence.

### Account Deletion

Account deletion requires re-authentication, deactivates the account immediately, and schedules asynchronous data purge.

## 5. Response Envelope

### Success

Single resource:

```json
{
  "data": {}
}
```

Collection:

```json
{
  "data": [],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
}
```

### Error

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "The request is invalid.",
    "details": [
      {
        "field": "email",
        "code": "INVALID_EMAIL",
        "message": "Enter a valid email address."
      }
    ],
    "request_id": "request-id"
  }
}
```

`details` is always an array. It MAY be empty when no field-level detail exists.

## 6. Error Model

The canonical error codes are:

| Code | Typical status | Meaning |
|---|---|---|
| `VALIDATION_ERROR` | 400 | Invalid request or field values |
| `UNAUTHENTICATED` | 401 | Missing, invalid, or expired credentials |
| `INVALID_CREDENTIALS` | 401 | Login or re-authentication failed |
| `NOT_FOUND` | 404 | Resource does not exist or is not owned by the caller |
| `CONFLICT` | 409 | State conflict |
| `VERSION_CONFLICT` | 409 | Journal version does not match the current server version |
| `IDEMPOTENCY_KEY_REUSED` | 409 | Same key used with a different request body |
| `ACCOUNT_DELETION_PENDING` | 409 | Account is already being deleted |
| `PRECONDITION_REQUIRED` | 428 | Required If-Match header is missing |
| `PAYLOAD_TOO_LARGE` | 413 | Request exceeds the allowed size |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | Unsupported content type |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `INTERNAL_ERROR` | 500 | Unexpected server error |
| `UPSTREAM_UNAVAILABLE` | 502 or 503 | AI or dependency unavailable |
| `UPSTREAM_TIMEOUT` | 504 | AI or dependency timed out |

Rules:

- Clients MUST branch on `code`, not on the human-readable message.
- Messages MUST be safe to display and MUST NOT reveal internal details.
- `request_id` MUST be returned for unexpected errors.
- Validation details MUST identify the field when possible.

## 7. Pagination

Collections use opaque cursor pagination.

Rules:

- Default limit: 20.
- Maximum limit: 100.
- Response exposes `next_cursor` and `has_more`.
- The response does not repeat the request limit.
- Ordering MUST be stable and documented per collection.
- Cursors MUST be opaque to clients.
- Deleted records MUST NOT reappear on later pages.
- Invalid cursors return `400 VALIDATION_ERROR`.

Journals order by `entry_date DESC, id DESC`.

Journal changes order by `updated_at ASC, id ASC` for synchronization.

Conversations order by `updated_at DESC, id DESC`.

Messages use a `before` cursor and return messages in ascending chronological order for rendering.

Memories and insights order by `created_at DESC, id DESC`.

## 8. Idempotency

Expensive or retry-sensitive writes use:

```http
Idempotency-Key: <uuid>
```

The OpenAPI contract marks which operations require the header.

Rules:

- The key is scoped to the authenticated user and operation.
- The same key with the same request body returns the original result.
- The same key with a different request body returns `409 IDEMPOTENCY_KEY_REUSED`.
- Keys expire after 24 hours.
- The backend MUST NOT create duplicate messages, jobs, or insights because of a retry.
- A retry after an AI timeout MUST reuse the same key.

## 9. Rate Limits

Initial limits are engineering defaults, not contractual quotas:

| Operation | Initial limit |
|---|---|
| Login | 5 per minute per IP and 10 per hour per account |
| Registration | 3 per hour per IP |
| Refresh | 30 per minute per IP |
| Journal writes | 60 per minute per user |
| Companion messages | 30 per minute per user |
| Insight generation | 5 per hour and 20 per day per user |

Rate-limited responses use `429 RATE_LIMITED` and SHOULD include `Retry-After`.

Limits MUST be enforced server-side. Client-side limits are only a UX aid.

## 10. Versioning and Deprecation

- The base path is `/api/v1`.
- Additive changes MAY be released within `v1`.
- Breaking changes require `/api/v2`.
- Deprecated operations SHOULD return `Deprecation` and `Sunset` headers.
- The mobile application MUST tolerate unknown response fields.
- Removing or renaming a field is a breaking change unless the field was documented as experimental.

## 11. AI Generation Behavior

### Companion Messages

Companion responses are synchronous.

The backend MUST:

- Persist the user message before or atomically with generation coordination.
- Use the `Idempotency-Key` header to prevent duplicate user messages.
- Apply explicit timeouts.
- Return a safe error if generation fails.
- Allow the client to retry with the same key.

The response MUST NOT expose:

- System prompts.
- Retrieved context internals.
- Provider request or response payloads.
- Token-level provider metadata beyond approved usage fields.

### Insight Generation

Insight generation is asynchronous.

The flow is:

1. Client submits a generation request with `Idempotency-Key`.
2. API returns `202 Accepted` with a job identifier and status URL.
3. Client polls the documented job-status operation.
4. Job completes with either an insight or an explicit `insufficient_evidence` outcome.
5. Client stops polling on a terminal state.

Polling guidance:

- Start after the server-provided `retry_after_seconds`.
- Exponential backoff up to 30 seconds.
- Stop after 10 minutes of foreground polling and show a manual refresh action.
- Stop polling when the application is backgrounded.
- Do not create a second job to replace a slow job.

When evidence is insufficient, the system MUST NOT fabricate an insight.

## 12. Health

Health endpoints are:

- `GET /health/live`
- `GET /health/ready`

Liveness confirms that the process is running.

Readiness confirms that the service can serve traffic and MAY check bounded dependencies such as the database and migrations.

Health responses MUST NOT expose secrets or internal infrastructure details.

## 13. Offline Journal Synchronization

The mobile client MAY maintain an encrypted local journal database and an outbox for offline writing.

Contract requirements:

- Every journal includes a monotonically increasing `version`.
- `PATCH /journals/{journalId}` and `DELETE /journals/{journalId}` require `If-Match` with the last known version.
- A missing `If-Match` returns `428 PRECONDITION_REQUIRED`.
- A stale version returns `409 VERSION_CONFLICT`.
- `POST /journals` requires `Idempotency-Key` so an offline create can be retried safely.
- `GET /journal-changes` returns journal changes ordered by `updated_at` and `id`, including tombstones.
- A deleted change has `deleted_at` set and `journal` set to `null`.
- The change cursor is opaque.
- The client MUST NOT silently overwrite a newer server version. It must preserve the local content as a conflict copy or ask the user to resolve it.
- Local journal content MUST be encrypted at rest.
- The encryption key MUST be stored in the platform secure store.
- Local journal data and keys MUST be removed on logout and after account deletion.
- Drafts MUST NOT be sent to AI processing until they are published and synchronized as active.

## 14. Contract Governance

Before an endpoint is considered implementation-ready:

- It exists in `openapi.yaml`.
- It has an `operationId`.
- It declares authentication requirements.
- It declares request and response schemas.
- It declares relevant 4xx responses.
- It has at least one realistic example.
- It passes OpenAPI 3.1 validation.
- It has a backend contract test.
- Mobile DTO fixtures are checked against the schema.

CI MUST run OpenAPI validation on every pull request.

## 15. Change Management

Changes affecting the following MUST be reviewed across backend and mobile:

- API contract.
- Authentication.
- Authorization.
- Database schema.
- Journal ownership and privacy.
- Memory extraction and retrieval.
- AI response behavior.
- Account deletion.
- User-facing product modules.

The backend repository owns the canonical shared contract. The mobile repository references it and MUST NOT redefine it.
