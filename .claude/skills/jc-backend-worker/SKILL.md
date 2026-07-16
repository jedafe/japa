---
name: jc-backend-worker
description: Use when creating or modifying Asynq background tasks/workers in the JapaConnect backend. Covers task definition, queue tier selection, idempotency, retry/backoff, payload versioning, and enqueuer wiring.
---

# JapaConnect Backend — Asynq Workers Skill

Three-tier queues on Valkey: **critical** (payments, auth emails, webhooks out) · **default** (search sync, notifications, media processing, general emails) · **low** (digests, cleanup, analytics rollups, backfills). ~68 registered tasks — keep `asynq-tasks.md` accurate.

## ⛔ WIRING GATE (BE-001 Anchor)
A task that is defined but not wired is worse than no task — it fails silently. Every new task PR must show all four:
1. Task type constant + payload struct + `NewXxxTask` constructor
2. Handler registered in the worker mux
3. Enqueue call site(s) using the REAL enqueuer (never noopEnqueuer outside tests)
4. Entry in `asynq-tasks.md` (type, queue, retry policy, idempotency note)
Missing any → STOP + alert listing what's unwired.

## Task Design Rules
- **Idempotent by default**: retries WILL happen. Guard with natural keys (`WHERE NOT EXISTS`), Valkey `SET NX` with TTL, or upserts. A task that sends an email must record send-state before/atomically-with sending.
- **Payload = IDs, not objects**: pass `user_id`, re-fetch fresh state in the handler (render-time-resolution thinking — state may have changed since enqueue). Exception: immutable event data.
- **Payload versioning**: additive changes only to payload structs; in-flight tasks from the previous deploy must still decode. Removing a field = two-deploy dance.
- **Enqueue after commit**: services enqueue side-effect tasks after the owning tx commits. A task must never observe (or act on) rolled-back data.
- **Queue tier by blast radius**: user-facing money/auth → critical; user-visible soon → default; nobody notices an hour's delay → low.
- **Timeouts + retries**: explicit `asynq.Timeout`, `asynq.MaxRetry`; exponential backoff default; tasks calling external services (Meilisearch, SeaweedFS, Honcho, mail) get shorter timeouts + more retries than DB-only tasks.
- **Context**: handlers respect `ctx` cancellation for graceful shutdown; long batch tasks checkpoint so a restart resumes, not repeats.

## Scheduled/Periodic Tasks
Registered via the scheduler with cron specs from config (not hardcoded) · overlapping-run protection (Valkey lock) for jobs that must be singletons · every periodic task also in the catalog.

## Failure Handling
Dead-letter queue monitored (Control Panel job monitor consumes these stats — the API it reads must reflect real Asynq inspector data) · retryable vs terminal errors distinguished (`asynq.SkipRetry` for permanent failures like validation) · GlitchTip capture on terminal failure with task type + payload IDs.

## Testing
Handler unit tests with fake stores · idempotency test: run handler twice, assert single side effect · payload round-trip encode/decode test · enqueue-site test asserting correct queue + options.
