# Section 11 -- Job Queue (Asynq): Implementation Complete

**Date:** 2026-06-27
**Section:** 11
**Phase:** 6
**Status:** Complete

## Overview

Implemented the Asynq job queue foundation for JapaConnect. The worker runs
alongside the HTTP server in `cmd/api/main.go` with graceful shutdown. Three
queue tiers (critical/default/low) provide priority separation. 38 task type
constants cover all planned async work across the platform. Placeholder handlers
are replaced per-section as features are built.

## Files Created (7)

| # | File | Purpose |
|---|------|---------|
| 1 | `internal/worker/config.go` | Three-tier Config from JC_WORKER_* env vars with sane defaults |
| 2 | `internal/worker/server.go` | NewServer, NewMux, Addr helpers for Asynq server setup |
| 3 | `internal/worker/client.go` | NewClient for enqueuing tasks from service layers |
| 4 | `internal/worker/tasks.go` | 38 task type constants across critical/default/low tiers, RegisterHandlers |
| 5 | `internal/worker/handlers.go` | 38 placeholder handlers (log and return nil), replaced per-section |
| 6 | `internal/worker/helpers.go` | getEnv and getEnvInt utilities |
| 7 | `internal/worker/worker_test.go` | 9 tests covering config, constructors, task uniqueness, queue distinctness |

## Key Decisions

1. **Three-tier queue** -- critical (payment webhooks, GDPR deletion), default
   (notifications, analytics flush), low (cache warming, cleanup). Exponential
   backoff per tier.

2. **Shared Valkey instance** -- worker uses the same Valkey as the cache layer.
   No separate Redis/Valkey deployment needed for queue backing.

3. **Placeholder handlers** -- all 38 handlers start as no-ops. Each section
   that needs async work replaces the relevant handlers (GDPR, analytics,
   notifications).

4. **Interface-based processor injection** -- GDPR and analytics use interface
   types (`GDPRProcessor`, `AnalyticsProcessor`) so the worker package can call
   service methods without importing those packages directly.

5. **Worker startup in main.go** -- Asynq worker launches in a goroutine alongside
   the HTTP server. Both share the same graceful shutdown signal.

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added StartWorker function, worker import |
| 2 | `cmd/api/main.go` | Launch Asynq worker alongside HTTP server with graceful shutdown |

## Test Results

9 tests covering:
- DefaultConfigReturnsSaneDefaults (queue names, concurrency ordering)
- NewServerDoesNotPanic (constructor safety)
- NewClientDoesNotPanic (constructor safety)
- NewMuxRegistersHandlers (handler registration)
- TaskTypeConstantsAreUnique (no duplicate task names)
- TaskTypeConstantsAreNonEmpty (all task names populated)
- QueueConstantsAreDistinct (critical/default/low are different)
- AddrFormatsCorrectly (host:port formatting)

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/worker/...` | ok -- 9 tests pass |
| `go build ./cmd/api/` | success |
| `go vet ./...` | clean |

## Task Type Registry (38 types)

Organized by queue tier:

**Critical Queue:**
- payment:webhook:process, gdpr:account:deletion:cleanup, gdpr:data:export

**Default Queue:**
- notification:dispatch_batch, notification:email, notification:push
- analytics:flush_cache, analytics:refresh_views, analytics:security_scan
- spam:cache:refresh, mention:resolve, content:score:recalculate
- follow:feed:fanout, media:process, media:cleanup
- blog:rss:cache, blog:featured:expiry

**Low Queue:**
- cache:warm, cache:invalidate, cleanup:expired_sessions
- cleanup:stale_feeds, cleanup:old_notifications
- ad:impression:aggregate, ad:click:aggregate, ad:analytics:daily
- forum:poll:close, forum:vote:recalculate
- professional:verification:expiry, comparison:share:cleanup
- review:aggregate:recalculate, review:helpfulness:score
- ticket:sla:check, ticket:auto:assign
- queue:metrics:collect

## Deferrals

- Asynqmon integration for live queue visualization in Control Panel
- Dead letter queue handling for permanently failed tasks
- Task priority within tiers (currently FIFO within each tier)
