# Beta Hardening — Local Backend

Date: 2026-11-21
Scope: backend and local staging operations.
Status: local beta-hardening checks implemented; hosted dashboards and clinical
review remain external gates.

This document closes the backend Phase 3 beta-hardening work for the local
staging path. GCP is intentionally deferred.

## 1. Health Gates

Command:

```powershell
.\scripts\staging-local.ps1 health
```

It checks:

- database connectivity,
- applied migration version,
- dirty migration state,
- pending/retry job backlog,
- failed jobs in the last 24 hours,
- due scheduled insights,
- AI calls and failures in the last 24 hours,
- monthly AI spend versus the global budget.

Exit behavior:

- `status=ok`: all hard gates pass.
- `status=warn`: non-blocking signals such as failed jobs or AI failure rate.
- `status=fail`: migration mismatch, dirty migration, backlog older than 10
  minutes, or due schedules that the scheduler has not processed.

Recommended alert cadence:

- every 5 minutes during active staging,
- every 15 minutes for a longer test window.

The command can be run by Windows Task Scheduler or cron. A non-zero exit code
is the alert.

## 2. AI Usage and Cost

Command:

```powershell
.\scripts\staging-local.ps1 usage
```

It reports, per day and operation:

- calls,
- input tokens,
- output tokens,
- estimated cost,
- failures.

The data comes from `ai_usage_events` and contains no prompt or journal content.
Account deletion removes usage events through the user foreign-key cascade.

Budget enforcement:

- per-user monthly budget,
- global monthly budget,
- scheduled generation returns `budget_exhausted` instead of calling the
  provider.

## 3. Backup and Restore

Take a backup:

```powershell
.\scripts\staging-local.ps1 backup
```

Verify a backup by restoring it into a disposable local PostgreSQL 17 +
pgvector container:

```powershell
.\scripts\staging-local.ps1 restore-check
```

The restore check:

- starts `pgvector/pgvector:pg17`,
- runs the PostgreSQL 17 `pg_dump` client against Neon,
- restores the custom-format dump,
- reads `schema_migrations`,
- counts public tables,
- removes the disposable container.

Verified result on 2026-11-21:

```text
backup created: backups/restore-check-20261121-182905.dump (0.06 MB)
restore verified: schema_version=17 public_tables=20
```

Backups are written under `backups/`, which is ignored by Git.

A general restore command is available:

```powershell
.\scripts\db-restore.ps1 `
  -DumpFile .\backups\<file>.dump `
  -TargetUrl <target-url> `
  -Force
```

The restore refuses a Neon production-looking target unless the database name
contains `restore` or `test`.

## 4. Incident Runbook

### API does not become ready

1. Check `.\scripts\staging-local.ps1 status`.
2. Check `docker compose -f compose.staging.yml logs --tail=100 api`.
3. Verify Neon connectivity and migration version.
4. Run `.\scripts\staging-local.ps1 health`.

### Worker backlog

1. Check `health` output for `jobs pending` and `oldest_pending_min`.
2. Inspect worker logs.
3. Confirm Ollama is running for embedding jobs.
4. Confirm the OpenRouter key and budget are valid.
5. Restart the worker:

```powershell
docker compose -f compose.staging.yml restart worker
```

### Scheduled insights are due

1. Check `health` for `due_schedules`.
2. Run one fan-out:

```powershell
.\scripts\staging-local.ps1 scheduler
```

3. Confirm the scheduler container is running.
4. Check `insight_schedules.next_run_at`.

### AI budget exhausted

1. Run `.\scripts\staging-local.ps1 usage`.
2. Check `AI_USER_MONTHLY_BUDGET_USD` and `AI_GLOBAL_MONTHLY_BUDGET_USD`.
3. Scheduled jobs should complete with `budget_exhausted`, not call the
   provider.

### Suspected data exposure

1. Stop the API and tunnel immediately.
2. Preserve logs without journal content.
3. Rotate JWT, rate-limit, database, and OpenRouter secrets.
4. Assess affected accounts.
5. Restore from the last verified backup if needed.
6. Document root cause and preventive actions.

## 5. Security Checklist

- [x] Journal and message content are not logged.
- [x] Secrets stay in `.env.staging` / `.env.staging.local`, both gitignored.
- [x] Account deletion and purge verified.
- [x] Sensitive memory excluded without consent.
- [x] Cross-user authorization covered by integration tests.
- [x] Refresh-token rotation and reuse detection covered.
- [x] Rate limits enforced server-side.
- [x] CI runs `govulncheck`, OpenAPI validation, unit tests, integration tests,
      and a container build.
- [ ] Local clinical review of crisis resources.
- [ ] Hosted secret manager and monitoring (deferred with GCP).
- [ ] External penetration test before public beta.

## 6. Rate-Limit Baseline

| Operation | Baseline |
|---|---|
| Login | 5/min per IP, 10/hour per account |
| Registration | 3/hour per IP |
| Refresh | 30/min per IP |
| Journal writes | 60/min per user |
| Companion messages | 30/min per user |
| Insight generation | 5/hour and 20/day per user |
| Account deletion re-authentication | 5/hour per user |

Tune after observing real staging traffic. Client-side limits are not a
security boundary.

## 7. Memory Staleness

Command:

```powershell
docker compose -f compose.staging.yml run --rm `
  --entrypoint /usr/local/bin/sela-ops api stale-memories -days 180
```

It reports the number of active memories whose last confirmation or update is
older than the threshold. It does not mutate data. Review stale memories before
adding an automatic reconfirmation policy.

## 8. Local Operations

```powershell
.\scripts\staging-local.ps1 up
.\scripts\staging-local.ps1 status
.\scripts\staging-local.ps1 health
.\scripts\staging-local.ps1 usage
.\scripts\staging-local.ps1 backup
.\scripts\staging-local.ps1 restore-check
.\scripts\staging-local.ps1 smoke
.\scripts\staging-local.ps1 tunnel
.\scripts\staging-local.ps1 down
```

Do not leave the API, worker, or scheduler running 24/7 on Neon Free Tier
unnecessarily. Start them for a test window and stop them afterward.

## 9. External Gates

These cannot be completed by code alone:

- Indonesian crisis-resource review by a named reviewer.
- Live model quality review with real, consented or synthetic cases.
- Public beta privacy policy, terms, and data-deletion disclosure.
- Hosted monitoring and secret management if GCP is later enabled.
- External penetration test before public launch.

## 10. Exit Criteria

Local backend beta hardening is complete when:

- [x] Health gates exist and return a non-zero exit code on failure.
- [x] AI usage and cost can be inspected without content.
- [x] Backup and restore are verified.
- [x] Incident runbook is documented.
- [x] Security checklist is documented and code checks pass.
- [x] Rate-limit baseline is documented.
- [x] Stale-memory count can be inspected.
- [ ] Crisis resources are clinically reviewed.
- [ ] Live model quality review is completed.
