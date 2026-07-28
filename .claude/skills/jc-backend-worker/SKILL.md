---
name: jc-backend-worker
description: Use when creating or modifying Asynq background tasks/workers in the JapaConnect backend. Covers task definition, queue tier selection, idempotency, retry/backoff, payload versioning, and enqueuer wiring.
---

# JapaConnect Backend — Asynq Workers Skill

Three-tier queues on Valkey: **critical** (payments, auth emails, webhooks out, university payouts) · **default** (search sync, notifications, media processing, general emails, university approval notifications) · **low** (digests, cleanup, analytics rollups, backfills, payout calculation, university logo fetch). ~72 registered tasks — keep `asynq-tasks.md` accurate.

## Japa University Tasks (ADR-063)
| Task | Queue | Idempotency | Notes |
|---|---|---|---|
| `university:process_enrollment` | critical | SET NX on `jc:uni:enrollment:processed:{payment_id}` | Creates/updates enrollment + writes `revenue_splits` row. Guard against double-credit. |
| `university:calculate_period_payouts` | low (scheduled) | Valkey lock per instructor+period | Aggregates open splits (`payout_id IS NULL`) into `instructor_payouts` once threshold met. |
| `university:send_instructor_payout` | critical | Advisory lock on instructor_id inside tx | Executes transfer via Section 17 provider; stamps splits with `payout_id` AFTER success. |
| `university:notify_instructor_approved` | default | Send-state guard | Email notification on instructor approval. |
| `university:notify_course_approved` | default | Send-state guard | Email notification on course marketplace approval. |
| `university:fetch_logo` | low | Natural key (university slug) | Downloads logo from seed URL → SeaweedFS → sets `logo` field. Used during university space seeding. |

**Revenue splits are immutable** — `university:process_enrollment` writes one row per revenue event and never updates it. A split correction is a compensating new row, never an edit. **Pro-stream splits** (`source='pro_stream'`) have no `payment_id` — they use a different idempotency key (`jc:uni:pro_stream:{instructor_id}:{enrollment_id}`) to avoid tripping the payment-id guard.

## Geo-Aware Tasks (ADR-064)
| Task | Queue | Notes |
|---|---|---|
| `maintenance:update_geoip_db` | low (scheduled, weekly Sun 02:00 UTC) | Downloads MaxMind GeoLite2, atomically swaps reader pointer. No restart needed. |

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
