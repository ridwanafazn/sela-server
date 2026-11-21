# Pre-public Backend

Date: 2026-11-21
Status: core pre-public backend implemented; payment provider and full admin
console remain external gates.

## 1. Scope

Implemented:

- email verification,
- password reset,
- session inventory and revocation,
- user data export,
- adult confirmation and legal acceptance,
- provider-independent entitlement state,
- manual support CLI for entitlements,
- email delivery abstraction for local file outbox and SMTP.

Not implemented:

- payment provider selection, checkout, webhook, invoice, and refunds,
- premium feature enforcement,
- admin role/API and support console,
- external identity verification for age,
- automated retention/export storage.

## 2. Email Verification

Endpoints:

- `POST /auth/email-verification/confirm` — public, token.
- `POST /auth/email-verification/resend` — authenticated.

Registration creates a 24-hour single-use token. Only its SHA-256 hash is
stored in `auth_tokens`. The raw token is sent by email.

Messages currently contain the raw token rather than a clickable link. A
client route and deep-link format can be added when the mobile client
implements the verification screen.

`REQUIRE_EMAIL_VERIFICATION` controls whether login is blocked for unverified
accounts. Default is `false` for local development. Set it to `true` in a
pre-public environment.

## 3. Password Reset

Endpoints:

- `POST /auth/password-reset/request` — public.
- `POST /auth/password-reset/confirm` — public, token plus new password.

Rules:

- Request always returns `202`, whether or not the email exists.
- Reset token lifetime is one hour.
- Tokens are single-use and hashed at rest.
- Confirming a reset revokes every active session.
- Password policy: 8–1024 characters.

## 4. Session Inventory

Endpoints:

- `GET /auth/sessions`
- `DELETE /auth/sessions` — revoke every session except the current one.
- `DELETE /auth/sessions/{sessionId}` — revoke one owned session.

Session responses expose ID, timestamps, expiry, revocation state, and whether
the session is the current one. Refresh-token hashes are never returned.

## 5. Email Delivery

Configuration:

```text
EMAIL_PROVIDER=noop|file|smtp
EMAIL_OUTBOX_DIR=var/mail
SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=
```

- `noop` discards messages and is suitable for tests.
- `file` appends JSON lines to `EMAIL_OUTBOX_DIR/outbox.jsonl` for local
  development. The file contains sensitive tokens and is gitignored.
- `smtp` sends through a relay using `net/smtp`.

Do not use `file` in production.

## 6. Data Export

Endpoint:

```http
GET /api/v1/me/export
```

Returns a JSON attachment with:

- account and profile,
- journals, including tombstones,
- conversations and messages,
- memories, sources, conflicts, and consent,
- insights, evidence, and feedback,
- per-entry journal analyses,
- insight schedules,
- AI usage metadata,
- session metadata without refresh-token hashes,
- entitlement.

The export excludes:

- password hashes,
- refresh-token hashes,
- auth token hashes,
- idempotency keys,
- internal job payloads.

Rate limit: `RATE_LIMIT_EXPORT_PER_HOUR_USER`, default `3`.

The export is synchronous. Large accounts may need asynchronous export and
object storage in a later phase.

## 7. Consent and Age Gate

Endpoints:

- `GET /me/consents`
- `PUT /me/consents`

Fields:

- `adult_confirmed`
- `terms_version`
- `privacy_version`

Rules:

- Adult confirmation is self-declared, not identity verification.
- Current legal versions come from `TERMS_VERSION` and `PRIVACY_VERSION`.
- A stale version is rejected.
- Sensitive memory consent remains separate at `/memory-consent`.
- The consent summary includes sensitive memory state.

## 8. Entitlements

Endpoint:

```http
GET /api/v1/me/entitlements
```

Returns:

- plan: `free` or `premium`,
- status: `active`, `past_due`, or `canceled`,
- current period end.

Defaults to `free`/`active` on first access.

No premium feature enforcement is implemented yet. That requires a product
decision on limits.

Manual support commands:

```powershell
go run ./cmd/ops user -email user@example.com
go run ./cmd/ops grant -email user@example.com -plan premium -days 30
go run ./cmd/ops revoke -email user@example.com
```

These are local DB-operational commands, not an admin API.

## 9. Security Notes

- Email and reset tokens are hashed at rest and single-use.
- Password reset revokes all sessions.
- Export excludes all credential material.
- Session inventory excludes refresh-token hashes.
- Email outbox files contain tokens and must never be committed.
- SMTP credentials belong in environment/secrets, never in source control.
- Production must set `REQUIRE_EMAIL_VERIFICATION=true` after delivery is
  verified.

## 10. External Gates

Before public paid launch:

- choose a payment provider (Midtrans, Xendit, or Play Billing),
- implement checkout and signed webhooks,
- define free/premium limits,
- implement entitlement enforcement,
- add admin roles and a support console if needed,
- replace self-declared age confirmation with the chosen legal policy,
- complete privacy policy, terms, and deletion disclosure,
- complete crisis-resource review,
- run an external penetration test.

## 11. Verification

Integration coverage:

- registration email verification token,
- unverified login blocked when required,
- verification token confirmation and reuse rejection,
- password reset request and confirmation,
- old password rejected and new password accepted,
- all sessions revoked after reset,
- session listing, current-session marking, single-session revocation, and
  revoke-others,
- consent acceptance and stale-version rejection,
- default free entitlement,
- data export content and absence of secret metadata.
