# Offline Sync Backend

Date: 2026-11-21
Status: backend endpoint implemented and tested.

This document defines the backend half of offline journal synchronization. The
mobile encrypted database, outbox, and conflict UI remain mobile work.

## 1. Endpoint

```http
GET /api/v1/journal-changes?cursor=<opaque>&limit=<1-100>
```

Authentication is required. Results are scoped to the authenticated user.

## 2. Ordering and Pagination

- Order: `updated_at ASC, id ASC`.
- Limit: default `20`, maximum `100`.
- The first request without a cursor starts from the oldest record.
- The cursor is opaque and encodes `updated_at` plus `id`.
- An invalid cursor returns `400 VALIDATION_ERROR`.
- `pagination.has_more` and `pagination.next_cursor` drive the next page.
- Continue until `has_more` is false, then persist the last cursor.

## 3. Response

```json
{
  "data": [
    {
      "id": "uuid",
      "version": 2,
      "updated_at": "2026-11-21T10:00:00Z",
      "deleted_at": null,
      "journal": {
        "id": "uuid",
        "title": "Title",
        "content": "Content",
        "entry_date": "2026-11-21",
        "status": "active",
        "version": 2,
        "created_at": "2026-11-20T10:00:00Z",
        "updated_at": "2026-11-21T10:00:00Z"
      }
    }
  ],
  "pagination": {
    "next_cursor": "opaque",
    "has_more": true
  },
  "sync": {
    "server_time": "2026-11-21T10:00:01Z"
  }
}
```

## 4. Tombstones

A deleted journal is returned as a tombstone:

```json
{
  "id": "uuid",
  "version": 2,
  "updated_at": "2026-11-21T10:00:00Z",
  "deleted_at": "2026-11-21T10:00:00Z",
  "journal": null
}
```

Rules:

- Delete increments `version`.
- `journal` is `null` for a tombstone.
- The client must remove the local record for that ID.
- Tombstones remain readable so a client that was offline can observe the
  deletion.

## 5. Drafts

Drafts are included in the changes stream with `journal.status = draft`.
The client may store drafts locally, but drafts are not sent to AI processing.

## 6. Conflict Rules

Writes remain online operations:

- `POST /journals` requires `Idempotency-Key`.
- `PATCH /journals/{id}` requires `If-Match` with the last known version.
- `DELETE /journals/{id}` requires `If-Match` with the last known version.
- Missing `If-Match` returns `428 PRECONDITION_REQUIRED`.
- Stale version returns `409 VERSION_CONFLICT`.

The backend does not silently overwrite a newer version. The client must keep a
conflict copy or ask the user to resolve it.

## 7. Mobile Sync Algorithm

1. Read the last saved cursor.
2. Call `GET /journal-changes` with that cursor.
3. Apply each record:
   - `journal == null`: delete the local record.
   - otherwise: upsert by `id`, never downgrading a newer local version without
     conflict handling.
4. Save `pagination.next_cursor`.
5. Repeat while `has_more` is true.
6. Sync on app foreground/resume.
7. Clear the local database and cursor on logout and account deletion.

## 8. What the Backend Does Not Do

- Encrypted local mobile database.
- Outbox persistence on the device.
- Automatic background sync.
- Conflict-resolution UI.
- Multi-device merge policy beyond optimistic concurrency.

Those remain mobile Phase 3 work.

## 9. Verification

Integration coverage:

- first page and cursor page,
- no duplicate records across pages,
- `updated_at` ordering,
- tombstone with `journal = null`,
- updated active record with incremented version,
- draft inclusion,
- cross-user isolation,
- invalid cursor returns `400`,
- `sync.server_time` present.

The existing index `journals(user_id, updated_at, id)` supports the query.
