# Phase 4B Backend - Privacy Center and Advanced Search

Date: 2026-11-21
Status: backend complete.

## 1. Scope

Phase 4B adds the backend-only depth items that can be implemented without
mobile, payment-provider, or push-provider credentials:

- privacy center and granular AI-context controls,
- automatic memory-extraction control,
- unified advanced search across journals, memories, insights, and analyses.

Deferred 4B+ items are listed in section 7 with their owning phase.

## 2. Privacy Center API

Endpoints:

```http
GET /api/v1/me/privacy
PUT /api/v1/me/privacy
```

Response fields:

- `ai_context_mode`: `none`, `journals`, or `journals_and_memories`,
- `memory_extraction_enabled`: boolean,
- `sensitive_memory_granted`,
- `adult_confirmed`,
- `terms_version`,
- `privacy_version`,
- `updated_at`.

Defaults preserve the pre-4B behavior: `journals_and_memories` and extraction
enabled. A missing row is created on first read or update.

## 3. Context Modes

| Mode | Journal retrieval and insight conversation evidence | Memory context |
|---|---|---|
| `none` | Disabled | Disabled |
| `journals` | Enabled | Disabled |
| `journals_and_memories` | Enabled | Enabled |

The active Companion conversation is always available. `none` disables
retrieved journal and memory context, not the messages in the open
conversation.

Enforcement:

- **Companion**: journals are retrieved only when allowed; memories are
  retrieved only in `journals_and_memories`. The active conversation remains
  available in every mode.
- **Insights**: `none` completes the job with `insufficient_evidence` before
  calling the provider. `journals` excludes memories from evidence.
- **Memory extraction**: `memory_extraction_enabled=false` skips automatic
  extraction for new journal and message jobs.
- **Per-entry analysis**: remains an explicit request for one entry and is not
  affected by automatic context mode.
- **Search**: always user-initiated and scoped to the caller's own records.

Changing the mode does not delete existing journals, memories, or insights.

## 4. Advanced Search API

Endpoint:

```http
GET /api/v1/search?q=...&types=journals,memories&limit=10
```

Rules:

- `q` is required and must be 2-200 characters.
- `types` is optional; defaults to all four types.
- `limit` is per type, 1-20, default 10.
- Results are ranked by PostgreSQL full-text rank, then date, then id.
- Snippets are plain text, capped at roughly 220 characters.
- Search includes drafts and tombstones are excluded.
- Sensitive memories are included because the caller owns them; the result
  carries `sensitivity` for client-side labeling.
- Cross-user isolation is enforced in every query.

Result fields:

- `type`: `journal`, `memory`, `insight`, or `analysis`,
- `id`,
- `parent_id` for analyses (the journal id),
- `title`, nullable,
- `snippet`,
- `date`,
- `status`, nullable,
- `sensitivity`, nullable,
- `score`.

Search is a discovery endpoint. It is not a paginated export or sync API.

## 5. Schema

Migration `20` adds `privacy_preferences`.

Migration `21` adds generated `search_vector` columns and GIN indexes to:

- `personal_memories`,
- `insights`,
- `journal_analyses`.

Journals already had a generated search vector and GIN index from migration
`5`.

## 6. Verification

Unit coverage:

- context policy mapping,
- unified type normalization,
- snippet bounds and plain-text output.

Integration coverage:

- privacy defaults and updates,
- invalid mode rejection,
- per-user isolation,
- memory extraction skipped when disabled and resumed when enabled,
- insight generation returns `insufficient_evidence` when context is `none`,
- unified search across all four content types,
- type filtering and per-type limit,
- short query, invalid type, and invalid limit rejection,
- cross-user isolation,
- journal deletion removes journal and analysis search hits,
- privacy preferences included in data export.

Local staging verification against Neon schema `21`:

- privacy defaults returned `journals_and_memories` and extraction enabled,
- update to `none` and extraction disabled persisted,
- search returned the journal while context mode was `none`,
- no memories were created while extraction was disabled,
- insight job completed with `insufficient_evidence` and no insight id,
- export included the privacy preferences.

## 7. Deferred 4B+ Items

| Item | Owner | Reason |
|---|---|---|
| Push notifications | Flutter + provider | Needs FCM/APNs credentials, device-token registration, and notification UX decisions. |
| Encrypted read cache | Flutter | Requires SQLite/SQLCipher, key storage, and retention policy on device. |
| Full offline-first | Flutter | Existing backend `journal-changes` and conflict rules are the server side; the outbox and conflict UI are mobile. |
| Multi-device sync | Flutter | Needs conflict-resolution UX and deletion behavior validation on devices. |
| Additional localization | Flutter + content | UI locale, copy review, and translated crisis resources are client/content work. |
| Memory graph | Product + backend | Needs a user-facing graph decision and retrieval evidence; conflict edges exist, but a graph API is not justified yet. |
| Cost and model optimization | Ops + product | Needs production usage data and a model-routing decision. |

## 8. Phase Position

The backend-only part of Phase 4B is complete. The remaining 4B+ candidates are
either Flutter-native or require external credentials/product evidence. Flutter
development can start without waiting for them; those items should be planned
inside the Flutter phase and revisited with real usage data.
