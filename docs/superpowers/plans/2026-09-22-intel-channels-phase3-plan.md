# Intel Channels Phase 3 (Push Adapters) Implementation Plan

**Goal:** Implement the three Phase-3 adapters per `docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md` §3/§5/§6/§7: `email` inbound, `webhook`/WebSub, and `search` monitoring — the last remaining crawl methods, completing the spec's adapter table.

**Architecture:** Push methods never poll. `email` deliveries arrive at `POST /api/v1/intel/inbound/email` (provider webhook, HMAC-verified); `webhook` channels subscribe to a WebSub hub and receive Atom/RSS pushes at `/api/v1/intel/inbound/websub/{channel_id}` (GET verify + POST delivery). Both write an `intel_inbound_events` row (Phase-1 table), dedupe via `jc:intel:inbound:idem:{hash}`, and enqueue a worker task that materializes a raw doc into the standard `detect_changes → pipeline` spine. `search` channels stay on the channel scheduler; `fetch_channel` redirects them to `intel:search_poll` (mirroring the `xhr_sniff` redirect), which runs the shared fetch path through a new `SearchFetcher` adapter emitting `application/x-ndjson` result lines.

**Tech Stack:** Go (chi, sqlc+pgx/v5, Asynq, pgxpool), PostgreSQL 18, Valkey, `internal/crypto` (AES-256-GCM secrets at rest — same payment-secret pattern as api_config). No new external deps: MIME parsing uses `net/mail` + `mime`/`mime/multipart`; WebSub is plain HTTP form posts.

**Status: complete.** Implemented: inbound email endpoint + `intel:inbound_email` MIME task, WebSub verify/push + subscribe/renew task chain, `SearchFetcher` (brave/google_cse/serpapi) + `intel:search_poll` redirect, email/websub/search curators, migration 000705 (`intel.email.signing_key`; the other four keys were already seeded by 000685), and all four catalog updates. `go build ./...`, `go vet ./...`, `go test ./internal/...` (114 packages incl. live-DB tests) green.

## Existing groundwork (Phase 1/2 — do not rebuild)

- `intel_inbound_events` table + `CreateIntelInboundEvent` / `MarkIntelInboundEvent` queries (migration 000681).
- `intel.inbound.manage` RBAC node seeded (000684).
- `ListDueIntelChannels` already excludes `manual`, `email`, `webhook`, `oneshot`, `xhr_sniff`.
- `HandleIntelFetchChannel` already redirects `xhr_sniff` channels to their task — the pattern `search` follows.
- `sources.CrawlMethodEmail/Webhook/Search` constants + `InboundMethod()` exist; `materialize.go` already defaults `webhook`/`search` channels to `pipeline='curate'`.
- Worker `intelChannelPayload` + `intelFetchChannel` shared fetch core (raw doc insert → detect_changes enqueue) is reused by `intel:search_poll` unchanged.
- `SiteURL` config (`JC_SITE_URL`) supplies the WebSub callback base.
- `internal/crypto.Encrypt/Decrypt` for `intel.search.api_key` + `intel.email.signing_key` (encrypted settings; decrypt failure = adapter disabled, never plaintext fallback).

## Global Constraints

- Layered architecture: Handler → Service → Store (sqlc only). Migrations via golang-migrate pairs (next number: **000705**).
- Every outbound HTTP request through `fetcher` safetransport (SSRF dial + redirect guard + body cap). Hub POSTs and SERP calls included.
- Inbound endpoints are unauthenticated by design (provider-facing) — security = HMAC signature verification + per-channel `allowed_senders`/secret + idempotency + body cap. They must NOT sit behind `RequireAuth`.
- `ErrAdapterDisabled` semantics preserved: unset `intel.search.provider`/`api_key` or missing email signing key → benign skip, no `consecutive_failures`.
- New task names, routes, Valkey keys, and settings land in the JEOS catalogs in the same change (Gate 2).
- Worker deps stay package-level via `RegisterIntelDependencies`; new ports declared locally in `internal/worker` (import cycle).
- Fan-out is fire-and-forget (Rule 8): inbound endpoints enqueue and return 202 immediately.

---

### Task 1: Inbound plumbing — queries, idempotency, raw-doc materialization

**Files:**
- Modify: `internal/db/queries/intel_channels.sql` → regen `internal/db/gen`
- Modify: `internal/intelligence/stores/stores.go` (wrappers)
- Create: `internal/worker/intel_inbound.go` (shared inbound helpers)

**Details:**

```sql
-- name: GetIntelInboundEvent :one
SELECT * FROM intel_inbound_events WHERE id = $1;

-- name: GetIntelEmailChannelByAddress :one
SELECT * FROM intel_source_channels
WHERE crawl_method = 'email' AND enabled
  AND lower(provider_config->>'inbound_address') = lower($1)
LIMIT 1;
```

- `intelChannelFromInbound` helper: resolve channel → source → create raw doc (`url` = stable per-delivery key like `email:{message-id}` / `websub:{channel_id}:{delivery_hash}`, `content_type` as delivered) → `MarkIntelInboundEvent(processed|failed)` → enqueue `intel:detect_changes`.
- Idempotency helper `intelInboundIdem(ctx, key string) bool`: `SET jc:intel:inbound:idem:{hash} 1 NX EX 86400`; nil rdb → not a duplicate (fail open, content_hash dedup still applies downstream).
- Headers persisted to `intel_inbound_events.headers` jsonb (bounded: cap entries at 64, values at 2KB).

---

### Task 2: `email` inbound endpoint + `intel:inbound_email` task

**Files:**
- Create: `internal/intelligence/handlers/sources_inbound.go` — `HandleInboundEmail`, `HandleWebSubVerify`, `HandleWebSubPush`
- Create: `internal/worker/intel_email.go` — `HandleIntelInboundEmail`
- Modify: `internal/server/server.go` (route mount — unauthenticated), `internal/worker/tasks.go`, `internal/worker/enqueuer.go`

**Details:**

- `POST /api/v1/intel/inbound/email` — Mailgun-compatible signature: fields `timestamp`, `token`, `signature` where `signature = hex(HMAC-SHA256(signing_key, timestamp+token))`. Signing key from `intel.email.signing_key` (crypto.Decrypt; unset → 503 `inbound_disabled`). Timestamp must be within ±5 min (replay guard).
- Body forms: `message/rfc822` raw MIME, or form fields `body-mime` / `stripped-text` + `subject` + `sender` + `recipient` (Mailgun forward-style posts).
- Channel resolution: `recipient`/`To:` localpart@domain → `GetIntelEmailChannelByAddress`; unknown recipient → 202 `{received:false}` (ack to stop provider retries) with event logged.
- Sender gate: channel `provider_config.allowed_senders` (exact or `*@domain`); platform fallback `intel.email.allowed_senders` (jsonb). Reject → event `ignored`.
- Idempotency on `sha256(channel_id + token|message-id)` → 202 dup.
- Task `intel:inbound_email` (critical queue): load event → MIME parse → prefer `text/plain` part, strip `text/html` fallback → raw doc (`content_type='text/plain'`, `url='email:'+message-id`, headers carry From/Subject/Date) → detect_changes.
- `email` curator: one record per delivery — `title`=Subject, `summary`=body text (truncated 4k), `article_url`/`external_url` first http(s) link found, `external_key`=`{channel_id}:{message-id}`. Pipeline default `curate` (materialize.go covers webhook/search; add `email` to `curateDefaultMethods`).

---

### Task 3: WebSub verify + push endpoints, subscribe/renew tasks

**Files:**
- Modify: `internal/intelligence/handlers/sources_inbound.go`
- Create: `internal/worker/intel_websub.go` — `HandleIntelWebSubSubscribe`, `HandleIntelWebSubRenew`
- New lease key `jc:intel:websub:lease:{channel_id}` (catalog)

**Details:**

- `GET /api/v1/intel/inbound/websub/{channel_id}` — params `hub.mode`, `hub.topic`, `hub.challenge`, `hub.lease_seconds`. Validate: channel exists, `crawl_method='webhook'`, `hub.topic` == `provider_config.topic`. `mode=subscribe` → write lease `{hub, topic, expires_at}` (TTL = lease_seconds) + echo `hub.challenge` (200 text/plain). `mode=unsubscribe` → delete lease + echo. Mismatch/absent challenge → 404.
- `POST /api/v1/intel/inbound/websub/{channel_id}` — read body (feed XML, cap via http.MaxBytesReader). When `provider_config.secret` set: require `X-Hub-Signature` `sha256=<hex HMAC(secret, body)>`; invalid → 401. Idempotency on body hash → inbound event → `intel:inbound_email`-style materialization task is NOT reused — websub goes straight to raw doc + detect_changes in a small `HandleIntelWebSubDelivery`? No — keep one worker task `intel:inbound_email` renamed generic? Decision: single shared task `intel:inbound_process`? — see note below. Simpler: the POST handler writes the inbound event and enqueues `intel:inbound_email`? No — names must be honest. **Final:** shared worker handler `HandleIntelInboundProcess` registered as task `intel:inbound_email`? Rejected — keep two thin tasks: `intel:inbound_email` (email MIME path) and reuse it is wrong for websub. → `intel:websub_delivery`? The spec's catalog only names `websub_subscribe`/`websub_renew`; deliveries can be processed inline in the HTTP handler (feed body → raw doc → detect_changes enqueue) since no external calls are needed — this matches fire-and-forget without inventing a task name. **Chosen:** inline processing in `HandleWebSubPush` (same helper as Task 1), no extra task.
- `intel:websub_subscribe` (low queue): load channel → `provider_config{hub, topic, secret?, lease_seconds?}` → safetransport POST to hub: `hub.callback={SiteURL}/api/v1/intel/inbound/websub/{id}`, `hub.mode=subscribe`, `hub.topic`, `hub.secret?`, `hub.lease_seconds?` → write `pending` lease. On 2xx enqueue `intel:websub_renew` via `ProcessIn(lease_seconds - margin)` (margin = max(lease/10, 1h)).
- `intel:websub_renew` (low queue): reload channel, re-run subscribe flow, re-enqueue itself for the next window. Channel disabled/deleted → stop the chain silently.
- Admin channel-create handler (`HandleAdminCreateChannel`): when `crawl_method='webhook'` and `provider_config.hub` present → enqueue `intel:websub_subscribe` (fire-and-forget).
- `webhook` curator: thin wrapper over the RSSNews item parser — WebSub pushes are Atom/RSS fragments (`Handles()='webhook'`).

---

### Task 4: `search` adapter + `intel:search_poll` + `search` curator

**Files:**
- Create: `internal/intelligence/fetcher/search.go` (+ `_test.go`)
- Create: `internal/worker/intel_searchpoll.go` — `HandleIntelSearchPoll`
- Create: `internal/intelligence/curator/search.go`
- Modify: `composite.go` (`pendingFetchMethods` → empty; dispatch `search`), `intelligence.go` (fetch_channel redirect for `search` channels)

**Details:**

- `provider_config{query, provider?, result_count?, recency?, engine_id?}`; settings `intel.search.provider` (brave|google_cse|serpapi), `intel.search.api_key` (crypto.Decrypt). Missing provider/key → `ErrAdapterDisabled`.
- Providers (single GET each, safetransport):
  - `brave`: `GET https://api.search.brave.com/res/v1/web/search?q=&count=&freshness=` header `X-Subscription-Token`.
  - `google_cse`: `GET https://www.googleapis.com/customsearch/v1?key=&cx={provider_config.engine_id}&q=&num=&dateRestrict=`.
  - `serpapi`: `GET https://serpapi.com/search.json?engine=google&q=&num=&api_key=`.
- Body = `application/x-ndjson`: `{"title","url","snippet","published_at?"}` per result — dedupe downstream by result URL (curator `external_key`).
- `result_count` cap 50; `recency` maps to provider freshness param (pd|pw|pm|py for brave; d/w/m/y for google; ignored serpapi).
- `intel:search_poll` (default queue): payload = `intelChannelPayload`; handler resolves channel+source → `intelFetchChannel` (raw doc + detect_changes reuse). `HandleIntelFetchChannel` redirects `search` channels → `intel:search_poll` (xhr_sniff pattern).
- `search` curator: NDJSON lines → records {title, external_url=url, summary=snippet}, `external_key` = result URL → re-polls dedupe.
- `pendingFetchMethods` → empty map; `PendingFetchMethod` retained for probe filters.

---

### Task 5: Migrations, catalogs, wiring, tests

- Migration `000705_intel_phase3_settings.up/.down.sql` — idempotent seed (same INSERT-ON-CONFLICT pattern as 000687): `intel.search.provider` (`""`), `intel.search.api_key` (`""`), `intel.email.inbound_domain` (`""`), `intel.email.allowed_senders` (`[]`), `intel.email.signing_key` (`""`).
- `asynq-tasks.md`: `intel:inbound_email` (critical), `intel:websub_subscribe` (low), `intel:websub_renew` (low), `intel:search_poll` (default).
- `api-routes-catalog.md`: `POST /api/v1/intel/inbound/email`, `GET|POST /api/v1/intel/inbound/websub/{channel_id}`.
- `valkey-keys-catalog.md`: `jc:intel:inbound:idem:{hash}` (24h), `jc:intel:websub:lease:{channel_id}` (lease TTL).
- `system-settings-catalog.md`: five new keys.
- `enqueuer.go`: taskDefaults + taskTier entries; `tasks.go`: constants + handlerRegistry entries.
- Tests: HMAC verify (good/bad/stale timestamp), recipient→channel resolution, sender gate, MIME multipart parse, idempotency skip; websub verify echo + topic mismatch, push signature + lease lifecycle, subscribe form POST (httptest hub); search provider contracts (httptest), `ErrAdapterDisabled` paths, `search` curator keys; fetch_channel→search_poll redirect; `go build ./... && go vet ./...` + `go test ./internal/...`.

### Risks

- Signature contract is Mailgun-shaped — other providers (Postmark/SES) deliver different signing schemes; the plan documents Mailgun-compatible as V1, extensible per provider key in the payload headers.
- WebSub hubs are third-party: subscribe POST must be best-effort (retry via asynq), lease renewal self-heals via re-enqueue chain.
- Search API costs money per query — `result_count`/`recency` bounded; `intel.search.api_key` unset = disabled by default.

---
