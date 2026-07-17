# Asynq Worker Audit — Full Project Sweep

**Date:** 2026-07-17 (working tree on `main` @ 516505f + uncommitted changes)
**Scope:** Every Asynq task in `internal/worker` + all producers/enqueue sites across `internal/` and `cmd/`, audited against the jc-backend-worker skill rules (wiring gate, idempotency, payload design, queue tiers, enqueue-after-commit, scheduler hygiene, failure handling, testing).
**Method:** Raw `grep`/`sed` byte verification (Read tool served stale bytes for `tasks.go`/`server.go` — the gap-audit memory warning reproduced). Live PostgreSQL used to verify JSONB semantics.

## Verdict

**59 task types registered, 59 handlers — but ~23 (39%) have no producer anywhere**, and **5 of the wired paths have producer/consumer payload contract mismatches that make them fail on every single run**. The two systemic causes: no payload round-trip tests (producers and consumers define payload shapes independently, in different packages), and a pervasive silent-degradation philosophy (nil enqueuer → drop, no SMTP → pretend success, noop processors) that hides all of it.

What's genuinely healthy: webhook delivery, notification batch dispatch, feed fan-out (ON CONFLICT idempotent ✓), redirect hit recording, moderation auto-hide, GDPR export/deletion enqueue chains, the payments enqueue-failure fix (**verified**: `ProcessWebhook` now returns the error → provider retries), and the new GDPR retention sweep wiring (all four gate points present).

---

## CRITICAL — features silently broken end-to-end

### C1. All auth transactional emails fail payload decode
`internal/auth/handlers.go:903-908` marshals `map[string]string{"html": "true"}` — a JSON **string**. `HandleEmailSend` (`internal/worker/handlers.go:405-410`) decodes `HTML bool`. `parsePayload` is plain `json.Unmarshal` (handlers.go:113) → `cannot unmarshal string into … type bool` on **every** `email:send` task from auth (verification, password reset). Task retries 25× then dies in DLQ. No email is ever sent.
**Fix:** send `"html": true` as a bool (use a typed payload struct shared with the handler) + payload round-trip test.

### C2. All wallet activity point credits fail payload decode
`internal/wallet/tasks.go:17-29` enqueues `{event_id, tenant_id}`. The **registered** handler `HandleWalletProcessActivityEvent` (handlers.go:1357-1383) requires `user_id` and `activity_key` — `scanUUID("")` errors → every task fails → DLQ. Meanwhile `wallet/tasks.go` contains `ProcessActivityEventTask`, a correct handler matching the producer payload (re-fetches the event by ID) — **it is never registered on the mux**. The wrong handler shadows the right one.
Additional defects in the registered handler even if payloads matched: no transaction / `pg_advisory_xact_lock` around read-balance→insert (same race as backend-audit High #6, duplicated at worker layer); duplicate delivery hits `idempotency_key` UNIQUE violation and retry-loops instead of skipping; `MarkEventProcessed` not atomic with the ledger insert.
**Fix:** register the wallet-service-backed handler (processor-adapter pattern like GDPR/webhooks), delete the inline duplicate, wrap in one tx with advisory lock, treat unique-violation as success.

### C3. Affiliate commission crediting is triple-broken (money path)
Producer `HandlePaymentWebhookProcess` (handlers.go:256-261) sends `amount` as string (`fmt.Sprintf("%d")` in `map[string]string`) and **omits `link_code`**. Consumer `HandleAffiliateCreditCommission` (handlers.go:3335+) decodes `Amount float64` (→ decode error, DLQ) and requires `LinkCode` for its member lookup (→ would no-op even if decoded). And its credit INSERT targets **`wallet_transactions` — a table that exists in no migration** (the wallet system uses `wallet_ledger` via `InsertLedgerEntry`). Zero commissions can ever be credited. No idempotency key on the credit either.
**Fix:** shared typed payload incl. link attribution resolved at enqueue time; credit via the wallet ledger with an idempotency key (`payment_id`-derived); reversal handling for refunds is still an open design gap.

### C4. Ticket confirmation & status emails go nowhere
`internal/tickets/service.go:625-655` enqueues `{ticket_id, tenant_id, subject}` and `{…, status, subject}`. Handlers require `email` as the SMTP recipient (handlers.go:353, :1172) and `new_status`/`old_status` (:1172-1174). Result: `sendEmail("", …)` → RCPT failure → 25 retries → DLQ (or silent nil if SMTP unset); status body renders "from  to ".
**Fix:** either include the recipient email at enqueue (fetch user at send-time is better — payload=IDs) or have handlers resolve the requester's email from `ticket.UserID`.

### C5. Scheduled publishing has a dead backstop and an unsafe primary
- Backstop: `HandleScheduledPublishSweep` (handlers.go:2631) selects `WHERE status = 'draft'`, but scheduled content is saved with `status='scheduled'` (`internal/content/handlers.go:562`). The sweep can never match anything.
- Primary: `enqueueScheduledPublish` (content/handlers.go:647-671) is fire-and-forget — enqueue failure is logged and dropped, and with the backstop dead, that content never publishes.
- `HandleScheduledPublish` (handlers.go:719-726) calls `UpdateContentStatus` **unconditionally** — no re-validation that the row is still scheduled → a schedule the author canceled (or content since unpublished/deleted) is force-published when the delayed task fires. Violates the re-fetch-fresh-state rule.
**Fix:** sweep matches `status='scheduled'`; publish task guards `WHERE status='scheduled' AND publish_at <= now()`; surfaces enqueue failure to the API response or relies on the (fixed) sweep.

### C6. `coupon:track_events` fails hourly, forever
Scheduled at :05 hourly (server.go:101). Handler (handlers.go:3193-3223) runs `UPDATE discount_codes SET uses_count = … WHERE ce.created_at > dc.updated_at`. `discount_codes` (migrations/000022) has **neither `uses_count` nor `updated_at`** — no later migration adds them. Every run is a SQL error × 25 retries → unbounded DLQ + GlitchTip noise. Conceptually it also conflates two systems: `coupon_events` belongs to the `coupons` table (000108, which has `total_clicks`/`total_copies`), not `discount_codes`.
**Fix:** rewrite against `coupons` counters (incremental, not `count-since-updated_at` overwrite), or drop the task if event rows are the source of truth.

---

## HIGH

### H1. Search indexing never happens
`search:index_entity` has zero producers. `search.NewIndexer` is registered as the worker processor (server.go:583-584) and that is its **only** consumer; there is no inline indexing on entity writes and no reindex path in Go code. Meilisearch is never populated/updated by the backend at runtime.

### H2. Media processing & virus scanning never run
`content:process_image`, `content:process_video`, `content:scan_media` have zero producers (`internal/media` contains no enqueue call). The ClamAV scan pipeline (`media/scanner.go`) is only reachable via the never-enqueued task → **uploaded files are never scanned**. Security-relevant.

### H3. DM push notifications are dead
`dm:notify_request` / `dm:notify_message` (critical tier) have zero producers — nothing in websocket/notifications enqueues them. The handlers correctly chain to `push:deliver`, but nothing ever starts the chain.

### H4. Feed velocity ranking reads a table nobody updates
Feeds queries rank by `content_scores.velocity_score` (`internal/db/queries/feeds.sql:22,82,110,139`). The only writer of that column is `maintenance:update_velocity_scores` — **which has no producer**. The scheduled `maintenance:recalc_velocity_scores` writes a different table (`content_engagement`) with a different formula. Net: "rising/velocity" ordering never changes from initial values.
**Fix:** pick one table+formula; schedule that one; delete the other handler.

### H5. `*/60 * * * *` fires hourly, not every 60 seconds
server.go:69 — robfig/cron v3 minute-field `*/60` matches only minute 0. The comment says "every 60 seconds". Once C5 is fixed the backstop sweep would still add up to 1h latency to missed publishes. Use `* * * * *` or `@every 60s`.

### H6. Zero timeout/retry policy discipline
No `asynq.Timeout`, `asynq.MaxRetry`, or `asynq.SkipRetry` anywhere in the codebase. All tasks: default ∞ timeout + 25 retries. `sendEmail` (handlers.go:2841+) uses `tls.Dial`/`smtp.SendMail` with no deadline and ignores `ctx` → one hung SMTP server can pin worker slots (critical tier only has 10). Malformed payloads (C1-C4) burn 25 retries over days instead of `SkipRetry` on first decode failure.
**Fix:** `parsePayload` failure → wrap with `asynq.SkipRetry`; per-task `Timeout` defaults in `taskDefaults()`; external-service tasks get short timeout + more retries per the skill.

### H7. Horizontal scaling multiplies every periodic job
The scheduler runs inside **every** `cmd/api` replica (cmd/api/main.go:80-85) with no leader election; `scheduledTask()` sets no `Unique`; no periodic handler takes a Valkey singleton lock (grep: zero lock usage in handlers.go). N replicas → N× GDPR sweeps, coupon expiry, prunes, all running concurrently. Even single-replica, a deploy overlap doubles them.
**Fix:** `asynq.Unique(period)` on scheduled tasks as cheap insurance + Valkey `SET NX` lock in sweep handlers, or run the scheduler in a dedicated singleton process.

### H8. Systemic silent-drop failure philosophy
`Enqueuer.Enqueue` returns nil when client is nil (enqueuer.go:37-39); `sendEmail` returns nil when SMTP unconfigured (prod misconfig = all email silently "succeeds"); noop processors registered as fallback (noop.go); feed fan-out logs-and-skips with no enqueuer. This is the exact failure mode the wiring gate exists to prevent — production misconfiguration is invisible to monitoring.
**Fix:** in production mode (env flag), nil enqueuer/SMTP/processors should be a startup error, not a graceful skip.

### H9. Newsletter send: wrong payload design + no send-state
Payload carries `subject`, full `html_body`, and the entire `recipients` array (newsletter/service.go:88-94) — violates payload=IDs (stale content, huge Valkey payloads). Handler (handlers.go:1496-1539) keeps `sent` only in memory: a crash mid-batch → retry resends to everyone; conversely all-recipients-failed still returns nil → silent loss. Ignores `ctx` → no graceful-shutdown checkpoint.
**Fix:** payload = campaign ID; per-recipient send-state rows (`WHERE NOT EXISTS` guard); chunked processing with checkpoints.

### H10. GDPR requests stuck forever on lost enqueue
`gdpr/service.go:193-196, 311-315` — enqueue failure for `data:export_user_data` / `account:deletion_cleanup` is logged only; the request row stays `pending` and nothing re-scans pending rows (the new retention sweep only covers `retention_wait`). A deletion request lost this way breaches the GDPR 30-day clock silently.
**Fix:** extend the retention sweep (or add a sibling) to re-enqueue stale `pending` requests older than N minutes.

---

## MEDIUM

- **M1. Retention sweep tenant gap** — handlers.go:293-301 INNER JOINs `system_settings` on `gdpr.retention_days`; the setting is seeded only for the default tenant (000137) → other tenants' `retention_wait` rows are never swept. Also relies on `value->>0` scalar-extraction quirk (verified working on live PG, but breaks silently if the value is ever stored as a bare number vs the service's `GetSettingInt` path — keep formats in lockstep).
- **M2. PII/secrets in logs** — nearly every handler logs the full payload at INFO: `email:send` logs complete bodies incl. verification/reset links (handlers.go:403), DM previews (:2571+), newsletter HTML. Log task type + IDs only.
- **M3. Payment webhook handler races** — full pending-payments scan matching `provider_ref == event_id` in Go (handlers.go:226-238); task has no `Unique`; concurrent duplicates can both update status (commission double-enqueue is saved only by the Unique default on the commission task). No status-transition guard (late `refunded` vs `succeeded` ordering).
- **M4. DLQ stats read the wrong field** — dlq.go:44-58 sums `qi.Failed` (daily failure **counter**, resets daily, includes retried attempts) as "DLQ depth". The dead-letter set is `qi.Archived`. Control-panel job monitor over-alerts on transient retries and misses actual dead tasks. Also calls `GetQueueInfo` twice per queue.
- **M5. Scheduler registration errors discarded** — every `scheduler.Register` is `_, _ =` (server.go:69-104); an invalid spec silently never runs. Log/fail on error.
- **M6. Cron specs hardcoded** — skill requires specs from config.
- **M7. Push delivery** — `push_subscriptions` query not tenant-scoped (handlers.go:2430); DB query error → `return nil` drops the notification silently.
- **M8. Embedding payload carries full text** (content/handlers.go:167-178) — stale-on-edit; payload should be IDs, handler re-fetches blocks.
- **M9. Dead code inventory** — `internal/tasks` package (zero importers; duplicate publish/velocity payload structs), `wallet.ProcessActivityEventTask` (unregistered — see C2), `feed/tasks.go:TaskUpdateVelocityScores`, `spaces` noop enqueuer + `EnqueueSpaceAutoPopulate` (zero callers, known), and the inline-superseded orphan tasks below.
- **M10. `asynq-tasks.md` catalog still doesn't exist** — the skill's gate point #4 is unenforceable without it. 59 types need cataloging (type, queue, retry policy, idempotency note).

## LOW

- `taskDefaults` comment references a "task-level Unique call" that doesn't exist; `idempotentTasks` map entries misaligned (gofmt).
- Two velocity formulas disagree (weights 1/5/10/3/8 vs 2/1/0.01) — resolve with H4.
- `HandleVelocityScoreRecalc` doc comment is duplicated/truncated (handlers.go:2699-2703).
- Queue tier questionables: `moderation:auto_hide` on **low** (user-harm mitigation waits behind digests); `ticket:send_confirmation` on **critical** vs `email:send` on default (inconsistent for same blast radius).

---

## Orphaned tasks (registered, zero producers) — 23 of 59

| Task | Why it matters | Disposition |
|---|---|---|
| `search:index_entity` | Search never updates (H1) | wire producers on entity writes |
| `content:scan_media` | No virus scanning (H2) | wire at upload |
| `content:process_image` / `content:process_video` | No async media processing (H2) | wire (imgproxy may make image task moot) |
| `dm:notify_request` / `dm:notify_message` | DM notifications dead (H3) | wire from DM message/request creation |
| `maintenance:update_velocity_scores` | Feeds ranking stale (H4) | consolidate with recalc task |
| `content:scan_moderation` | Moderation scan pipeline unused | wire at content create/update |
| `content:fetch_link_preview` | Previews never fetched async | wire from block editor flow |
| `ai:process_task` | AI queue processing dead | wire |
| `i18n:translate_language_pack` | Admin translate feature has no trigger | wire from admin i18n endpoint |
| `learning:update_track_completion` / `learning:issue_certificate` | Progress/certificates never issued async | wire from lesson-completion flow |
| `email:send_digest` / `newsletter:digest` | Digest features dead (also unscheduled) | schedule + wire |
| `sitemap:generate` | Never triggered (sitemap served on-request today) | schedule |
| `ad:ingest_event_batch` | Ad events never ingested | wire |
| `maintenance:update_geoip_db` | GeoIP DB never refreshed | schedule it |
| `db:manual_vacuum` | No control-panel trigger exists | add control route |
| `space:auto_populate` | Known dead (audit #17) | per prior decision |
| `content:update_engagement` / `content:recalc_review_aggregate` / `affiliate:record_click` | Superseded by inline paths | **delete** (dead weight) |

## Skill-rule compliance scorecard

| Rule | Status |
|---|---|
| Wiring gate (4 points per task) | ❌ 23 orphans; 5 wired-but-contract-broken; catalog missing |
| Idempotent by default | ⚠️ fan-out ✓, publish-sweep guard ✓, GDPR ✓; wallet/commission/newsletter/emails ❌ |
| Payload = IDs | ❌ newsletter (full content), embedding (full text), email (full body), push (full text) |
| Payload versioning | ⚠️ untestable — no shared payload types at all (producers/consumers drift freely; C1-C4 are the proof) |
| Enqueue after commit | ✓ spot-checked payments/gdpr/tickets/webhooks/feed — no pre-commit enqueues found |
| Queue tier by blast radius | ⚠️ mostly sane; auto-hide on low, ticket email on critical |
| Explicit Timeout + MaxRetry | ❌ zero usage anywhere (H6) |
| Ctx cancellation / checkpoints | ❌ email/newsletter handlers take `_ context.Context`; no batch checkpoints |
| Scheduler: cron from config | ❌ hardcoded (M6) |
| Scheduler: singleton locks | ❌ none (H7) |
| DLQ monitored w/ real inspector data | ⚠️ endpoint exists but reads wrong field (M4) |
| SkipRetry for terminal errors | ❌ zero usage (H6) |
| GlitchTip on terminal failure | ✓ server-level ErrorHandler wired (fires every attempt, not just terminal — acceptable) |
| Tests: handler unit / idempotency / round-trip / enqueue-site | ❌ none of the four exist (`worker_test.go` is config-only) |

## Recommended fix order

1. **C1 + C4** — auth + ticket emails (shared typed payload structs in `internal/worker`, imported by producers; SkipRetry on decode failure). Smallest diffs, biggest user impact.
2. **C2** — register the wallet service handler, delete the inline duplicate, tx+lock.
3. **C3** — commission payload + wallet_ledger credit with idempotency key.
4. **C5 + H5** — publish status guards + cron spec.
5. **C6** — coupon tracker rewrite or removal.
6. **H6/H7** — `taskDefaults` gains Timeout/MaxRetry; decode-failure → SkipRetry; Unique on scheduled tasks.
7. **H10 + M1** — GDPR pending sweep + per-tenant settings fallback.
8. **Testing debt** — payload round-trip tests for every producer/consumer pair (would have caught C1-C4), idempotency double-run tests, enqueue-site queue/option assertions.
9. **M10** — write `asynq-tasks.md` (blocked on deciding orphan dispositions above).
10. Orphan cleanup per the table — each is a product decision (wire vs delete), flag to owner.

**Coverage note:** all 59 handlers were inventoried and cross-referenced for producers; handler *bodies* were read line-by-line for the live/critical paths (payments, wallet, emails, newsletter, publish, fan-out, push/DM, GDPR sweep, coupons, affiliate, redirect, velocity, DLQ). Orphaned-task bodies (digests, sitemap internals, SLA sweep, prunes, ad ingest, learning, GeoIP, vacuum, security scan) were skimmed for registration only — their internals are moot until wired, but should be re-reviewed at wiring time.
