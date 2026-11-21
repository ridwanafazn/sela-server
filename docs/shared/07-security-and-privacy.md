# SELA Security and Privacy

Version: 1.3

> Scope note (2026-11-21): sensitive memory requires explicit, recorded, and
> revocable user consent. The default remains skip. See
> `10-product-vision-decisions.md`.

## 1. Security Objective

SELA handles personal journals, conversations, preferences, and inferred memories.

Confidentiality and user control are core product requirements.

Security must be considered throughout:

- Collection.
- Transmission.
- Storage.
- Processing.
- Retrieval.
- AI provider interaction.
- Retention.
- Deletion.

## 2. Data Classification

### Public

Information intended for public distribution.

### Internal

Operational information not intended for public distribution.

### Confidential

Personal account information and non-public user data.

### Highly Sensitive

Personal journals, private conversations, sensitive memories, and other intimate personal content.

Highly sensitive data requires stricter access and logging controls.

## 3. Authentication

Requirements:

- Secure password hashing using a modern password-hashing algorithm.
- Short-lived access tokens.
- Refresh-token rotation or equivalent session protection.
- Refresh-token hashes stored instead of raw tokens.
- Session revocation.
- Login rate limiting.
- Secure error messages that do not reveal account secrets.
- Email verification and password-reset tokens are hashed at rest, single-use,
  and time-limited.
- Password reset revokes every active session.

## 4. Authorization

Every user-owned resource must be scoped to the authenticated user.

Examples:

- Journals.
- Conversations.
- Messages.
- Memories.
- Insights.
- Preferences.

Authorization must be enforced on the backend.

Do not trust ownership identifiers supplied by the client.

## 5. Transport and Storage

- Use HTTPS in production.
- Use managed secrets rather than hardcoded credentials.
- Restrict database access.
- Apply least-privilege permissions.
- Protect backups.
- Avoid unnecessary copies of sensitive content.
- Review encryption capabilities of each infrastructure provider.

## 6. AI Provider Privacy

Before sending user content to an external AI provider:

- Identify what data is required.
- Minimize unnecessary context.
- Review provider data-retention policies.
- Review provider training and data-use policies.
- Use contractual and configuration protections where available.
- Avoid sending unrelated journal content.

Provider selection must consider privacy, cost, reliability, and quality.

## 7. Prompt Injection

Retrieved journals and memories are untrusted content.

A journal may contain instructions that attempt to override system behavior.

The AI system must:

- Treat retrieved content as evidence.
- Not treat retrieved content as system instructions.
- Preserve system and developer instruction boundaries.
- Validate structured output.
- Enforce authorization outside the model.

The model must never be the sole authorization mechanism.

## 8. Logging

Allowed operational metadata:

- Request ID.
- User ID or pseudonymous internal identifier where necessary.
- Endpoint.
- Status code.
- Latency.
- Provider name.
- Model identifier.
- Token usage.
- Error category.
- Job status.

Do not log full journal bodies or full private conversations by default.

Sensitive debugging must use controlled access, minimization, and a defined retention period.

## 9. User Data Control

Users should be able to:

- Inspect personal memories.
- Correct memories.
- Reject memories.
- Delete memories.
- Delete journals.
- Delete conversations.
- View and revoke sessions (current session in MVP; full session inventory and
  revoke-all are post-MVP).
- Delete their account.
- Export their data without credential material.
- Inspect adult confirmation and legal acceptance state.

The system must explain relevant consequences of deletion.

## 10. Data Deletion

Account deletion must account for:

- Profile.
- Journals.
- Conversations.
- Messages.
- Memories.
- Insights.
- Insight feedback.
- Embeddings.
- Sessions.
- Idempotency records.
- User-owned jobs.

Account deletion is an MVP requirement, including for Google Play compliance when the application supports account creation.

- The API requires re-authentication.
- The account is deactivated immediately.
- A durable purge job removes user-owned data asynchronously.
- The client must clear local session state after a successful deletion request.

Deletion must also consider external provider retention and backup retention.

Do not promise immediate deletion from backups unless the infrastructure process guarantees it.

## 11. Sensitive Memory Handling

Sensitive information is skipped by default. It must not be extracted into
long-term memory without explicit, recorded, and revocable user consent.

With consent:

- Store only the minimum necessary sensitive memory.
- Retain explicit source references and confidence.
- Restrict retrieval to operations covered by the consent.
- Allow the user to inspect, correct, reject, and delete the memory.
- Allow the user to revoke consent at any time.
- Re-evaluate or delete stale sensitive memories.
- Do not use sensitive memories for model training.

Consent must be specific. It must not be bundled into a generic "accept all"
action. When consent is absent, expired, or revoked, the sensitive memory must be
excluded from retrieval and AI context.

The MVP exposes `GET /memory-consent` and `PUT /memory-consent` for the
sensitive-memory scope. Memory responses include a `sensitivity` field.

Do not infer sensitive identity attributes without an explicit, justified need.

## 12. API Security

Apply:

- Input validation.
- Request size limits.
- Rate limiting.
- Authentication middleware.
- Ownership checks.
- Safe error handling.
- Dependency scanning.
- Security testing.

Protect against:

- Broken access control.
- Injection.
- Credential leakage.
- Abuse of AI endpoints.
- Denial of service.
- Excessive resource consumption.

## 13. Infrastructure Security

- Restrict service account permissions.
- Keep secrets outside source control.
- Protect CI/CD credentials.
- Separate development and production.
- Restrict database access.
- Review Cloud Run ingress and invocation settings.
- Monitor suspicious access patterns.

A public mobile API may require public Cloud Run invocation, but application authentication and ownership authorization remain mandatory.

## 14. Incident Response

The incident process should include:

1. Identify the incident.
2. Contain the affected component.
3. Assess data exposure.
4. Preserve relevant operational evidence.
5. Restore service safely.
6. Notify affected parties when required.
7. Document root cause.
8. Implement preventive actions.

## 15. Safety Boundaries

SELA is not a clinical service.

It must not:

- Diagnose users.
- Replace professional care.
- Encourage self-harm.
- Shame users.
- Present unsupported psychological interpretations as facts.

When serious distress or immediate danger is indicated, prioritize safety and appropriate real-world assistance.

## 16. Security Review Checklist

Before production:

- Authentication tested.
- Authorization tested.
- Cross-user access tested.
- Token revocation tested.
- Sensitive logs reviewed.
- Secrets reviewed.
- Database permissions reviewed.
- AI data-sharing reviewed.
- Deletion tested.
- Rate limits configured.
- Dependency vulnerabilities reviewed.
- Incident process documented.
- Sensitive memory consent tested, including revocation and exclusion from
  retrieval.
- Crisis resources reviewed by a named reviewer before real journal data.
- Data export, age gating, and consent-center scope decided before public
  launch.
