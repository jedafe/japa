# JapaConnect Bug Hunter's Guide

JapaConnect (`github.com/jedafe/japa`) is a multi-tenant Go monolith (chi router, sqlc/pgx,
PostgreSQL 18, pgvector) serving several frontends: a SSR main app (`apps/main`, port 3000),
admin and control SPAs (`apps/admin` 3001, `apps/control` 3002), plus docs and learn surfaces.
It layers background jobs (Asynq three-tier queues), Valkey caching, Meilisearch, WebSocket
realtime, a Tiptap/Page-Builder content stack, a multi-provider AI gateway, payments/wallet,
RBAC with dynamic permission nodes, runtime i18n, and GDPR/NDPR erasure.

**The single source of business truth is the `.jeos/` repo** (JEOS): `JEOS.md` index, `sections/*`
(per-feature schema + routes + logic), `conventions/*` (api-response-format, api-routes-catalog,
permission-key-naming, system-settings-catalog, valkey-keys-catalog, asynq-tasks,
database-migration-naming, feature-flag-naming, svelte-component-patterns), the 60 ADRs, and the
`AGENTS.md` **17 Golden Rules**. When hunting bugs, a route/field/handler that contradicts JEOS is
a bug even if it "compiles and runs."

Legend: `[BE]` backend-only · `[FE]` frontend-only · `[BOTH]` either.

---

## 1. Functional Bugs

### 1.1 Form & Validation Errors `[BOTH]`
- [BE] Handler accepts a payload the store layer rejects later, surfacing a raw 500 instead of a
  structured error envelope (`{ "error": { "code": "snake_case", "message": "..." } }`, Rule 5).
- [FE] Submit button not disabled during in-flight request → double-submit creates duplicate forum
  posts, payments, or ticket replies. SPA must guard with a `submitting` `$state`.
- [FE] Zod/client validation passes but server sqlc validation fails (mismatched constraints).
  Keep client + DB constraints in sync.
- [FE] Bits UI `Dialog`/`Dropdown`/`Select` controls not bound to `$state` → values never sent.
  Use `bind:value` / callback props, never `bind:` on slots.

### 1.2 Navigation & Routing `[FE]`
- [FE] SvelteKit client nav uses `window.location` redirects instead of `goto()` → full reload
  kills SPA auth/session context.
- [FE] Page placed outside the `(auth)`/`(app)` route group loses the layout/auth guard → silent
  unauthenticated access or broken shell.
- [FE] `window.history` back restores a list page but DataTable server-side sort/filter (querystring
  source of truth) is lost.
- [BE] chi route registered with trailing-slash mismatch or wrong method → 404/405 only in prod.

### 1.3 Calculation Errors `[BE]`
- Wallet/credits ledger (`internal/wallet`) double-counts a debit/credit on retry → balance drift.
  Every ledger write must be idempotent (idempotency key or unique `(txn_ref)`).
- Coupon/affiliate mis-applies discount stacking; money math must use integer minor units, never float.
- Points/karma awarded twice via duplicate Asynq tasks.
- Tax/fee computed with stale currency/rate `system_settings`.

---

## 2. UI/UX (Visual & Usability) Bugs `[FE]`

### 2.1 Responsive Design Flaws
- Text overflow in the X.com-style unified app shell (`ADR-058`) on mobile; left nav / right TOC
  collapse states missing for the three documented exceptions (lesson player, API full-width).
- LayerChart (`2.0.1`) charts overflow container on narrow viewports; `Tooltip` snippet off-screen
  on touch. **Never use `xNice` on a `BarChart` band scale** — it collapses to 0–1; use `xPadding`.
- Bits UI popover/tooltip clipped by `overflow-hidden` ancestors.

### 2.2 Z-Index / Overlay Issues
- Modal `Dialog` behind sticky top bar (Tailwind z-index token collision). Bits UI portals to body.
- `Dropdown`/`Select` menu not clickable when a sibling `Tooltip`/overlay intercepts pointer events.
- Dark-mode (`[data-theme='dark']`, toggled via `jc_theme` cookie) breaks because a raw hex was used
  in a Tailwind class instead of a CSS custom property. **Never use raw hex in Tailwind classes.**

### 2.3 Broken Assets & FOUF
- Unstyled flash because theme tokens load after hydration; fonts/`$t` strings pop in.
- Missing media from `internal/media` (upload library) → broken `<img>` when CDN/Valkey-cached URL 404s.

---

## 3. Integration & API Bugs

### 3.1 Failed API Calls `[BOTH]`
- [FE] API client 401 handler does `window.location.href = '/login'` with token in `localStorage` →
  infinite redirect loop. Handle 401 by clearing token state locally.
- [FE] SPA fetches on mount but ignores `jc_session` cookie; SSR uses `$page.data.session` → 401 on
  first load that works after manual refresh.
- [BE] Handler returns empty payload for "no rows" instead of `[]` / `null` the frontend expects.

### 3.2 Sync / Consistency `[BE]`
- Payment (`internal/payments`, provider abstraction + manual transfer `ADR-012`) charges but webhook
  (`internal/webhooks`) fails to update order/subscription → orphaned paid order.
- Notification batching/dedup (`ADR-035`) drops or duplicates notifications when dedup key collides.
- Activity-feed fanout (`ADR-032`) writes to wrong recipient timeline.

### 3.3 SPA ↔ Backend Contract Drift
- DataTable composite keys `${id}::${index}` must be stripped to the bare id in **every** API call and
  filter comparison. Forgetting the `extractId` helper → bulk actions target `id::0` and 404.
- Response field renamed/removed (sqlc struct change) but frontend still reads the old field → silent
  `undefined` → empty column.
- Polymorphic feed items must include the discriminator (`content_type`, `purpose_type`) at **top
  level**, not nested (`conventions/api-response-format.md`).

---

## 4. Performance Bugs

### 4.1 Query & Latency `[BE]`
- N+1 inside a store loop (per-post author/reactions). Use sqlc batch queries or pgx `Batch`.
- Missing index on hot paths (follows, notifications, search filters) → seq scans under tenant load.
- `pgvector` similarity without IVFFlat/HNSW index or wrong probe count → full-table scan.

### 4.2 Caching `[BE]`
- Valkey key not tenant-scoped with `:{tenant_id}` → one tenant sees another's data. Known flat-key
  offenders: `jc:llms_txt`, `jc:llms_full_txt`, `jc:robots_txt` (must be `jc:llms_txt:{tenant_id}`).
- Cache stampede on hot key expiry → singleflight/lock required.
- Stale cache not invalidated after a mutation (e.g., profile update still shows old avatar).

### 4.3 Memory / Leaks `[BOTH]`
- [BE] Asynq worker / WebSocket hub (`internal/websocket`) leaks goroutines/connections on disconnect.
- [FE] SPA polling (`apps/control`) interval never cleared on destroy (`$effect` without cleanup).
- [FE] `$derived` closing over stale `$state` (`.find()` inside derived) → UI never updates.

### 4.4 Search `[BE]`
- Meilisearch (`ADR-017`) index out of sync after a write (not updated in same transaction boundary).
- Unbounded search query string; missing `filter` on tenant.

---

## 5. Security Bugs

### 5.1 Injection & XSS `[BOTH]`
- [BE] Any raw `fmt.Sprintf` into SQL instead of sqlc binding → SQL injection. Hand-written store SQL
  is the risk surface.
- [FE] Tiptap / Page-Builder HTML rendered with `{@html}` without server-side sanitization → stored
  XSS. Sanitize in `internal/renderer`, never trust client HTML.
- [FE] User content interpolated into `href`/`src` → `javascript:` URI XSS.

### 5.2 Auth, Sessions & CSRF `[BE]`
- `jc_session` cookie not httpOnly / not validated for tenant → fixation or cross-tenant access.
- Mutations missing CSRF token from `$page.data.csrfToken` → CSRF.
- 2FA (`pquerna/otp`) recovery path not rate-limited → brute force.

### 5.3 RBAC & Multi-tenancy Escapes `[BE]`
- `hasPermission()` node invented ad hoc instead of aligning to the catalog / sidebar nav map → either
  a 403 where the UI shows the button, or an unguarded mutation. (See §26 for the full RBAC-wiring bug
  class.)
- Tenant filter missing on a store query → cross-tenant read/modify.
- Impersonation (`ADR-047`) not terminated correctly → admin retains user powers.

### 5.4 Secrets & Config `[BE]`
- `PUBLIC_JC_*` (client) vs `JC_*` (server-only) leakage. A server secret in a `PUBLIC_` var ships to
  the browser bundle.
- Hardcoded credentials/keys committed; secrets stay out of VCS and the build.

---

## 6. Syntax & Logic Errors

### 6.1 Svelte 4 → 5 Anti-patterns `[FE]` (forbidden syntax)
- `export let` → `$props()`; `$:` → `$derived`/`$effect`; `on:click` → `onclick`;
  `createEventDispatcher` → callback props; `$store` outside `.svelte` → `get()`.

### 6.2 Go Logic Errors `[BE]`
- Error not wrapped with `%w` → `errors.Is/As` fail in middleware.
- Map/struct zero-value bugs: `nil` slice serialized as `null` not `[]`; pointer field left nil → NPE.
- Loop-variable capture in goroutines (pre-Go 1.22) in worker fan-out → wrong tenant/user id.
- Transaction boundary wrong: commit before dependent write, or long txn holding locks during an
  external API call → deadlock / partial commit.

### 6.3 Runtime Errors `[BOTH]`
- [FE] `{#each}` duplicate keys throw `each_key_duplicate` even when the natural key legitimately
  repeats (same `task_id` in different states). Use composite `${id}::${index}`.
- [BE] Nil deref on optional DB column scanned into a non-pointer field; sqlc nullable columns must
  map to `pgtype.*` / pointer types.

---

## 7. Translation / i18n Errors `[FE]`

Runtime i18n (`ADR-013`) — strings resolved at request time, not compile time.
- Missing `$t()` key → key shown literally (e.g., `users.create.title`).
- `en.json` has the key but `fr.json` empty/absent → blank UI in FR.
- Key present in JSON but never wrapped in `$t()` → hardcoded English leaks.
- Interpolated `$t('x', {n})` with missing params renders `undefined`.
- New page adds visible strings without adding keys to **both** `en.json` and `fr.json`.
- `i18n.default_language` / `i18n.enabled_languages` (`en,fr`) not honored → locale switch broken.

---

## 8. Database Migration & Schema Bugs `[BE]`

golang-migrate, `migrations/000001..`. Up/down must pair and apply with `make migrate`.
- Up applied but down not reversible → broken older revision.
- Circular FK created without the **stub pattern** (create `spaces` stub → create `users` → ALTER add
  creator FK, Rule 13) → migrate fails on empty DB.
- NOT NULL column added without backfill default → existing rows reject the migration.
- Locking ALTER on a hot table without `CONCURRENTLY` / zero-downtime rule → prod write stall.
- Enum/type added in up but not dropped in down → drift between `sqlc.yaml` models and DB.
- After a new migration, `sqlc` models not regenerated → store compiles against stale struct. (Never
  hand-edit `internal/db/` — Rule 2; edit `db/queries/`.)
- Soft-delete vs hard-delete confusion: a query forgets `WHERE deleted_at IS NULL` and returns
  anonymized/erased rows (see §15, Rule 11).

---

## 9. RBAC & Multi-tenancy Bugs `[BE]`

- Permission node added to the backend route but not to the catalog / sidebar map → UI hides a feature
  an admin should see, or shows one they cannot use.
- `hasPermission()` checks the wrong node name (typo) → always 403 or always 200.
- Tenant-scoped Valkey/Meilisearch keys built without `:{tenant_id}` → cross-tenant data bleed (§4.2).
- Background worker runs without a tenant context → writes to tenant `0` / wrong shard.
- Feature flag (`internal/features`) gating a feature but the flag check missing on one code path.

---

## 10. Background Jobs / Asynq / Worker Bugs `[BE]`

Three-tier queue (`ADR-016`), 59 registered tasks (`conventions/asynq-tasks.md`).
- Task name used that is **not** in the Section 11 / asynq-tasks catalog → silently dropped, never
  processed (Rule 7).
- Not idempotent → duplicate task (retry/backoff) double-applies a side effect (email twice, points
  twice, post published twice).
- Retry/backoff misconfigured → poison task hammers a failing external API; should land in **DLQ**
  (`apps/control` `/dlq`).
- Payload not versioned → worker can't decode old queued payloads.
- Cron job duplicate-runs across instances without a leader lock.
- Worker panic not recovered → Asynq server crashes; missing `recover` middleware.
- Fan-out task (`content:fan_out_to_feeds`) blocks the request handler instead of fire-and-forget
  (Rule 8).

---

## 11. Real-time / WebSocket Bugs `[BE/FE]`

`internal/websocket` (gorilla/websocket) for DMs, presence, notifications.
- Connection not re-established on network drop → silent loss of DMs/notifications.
- Per-connection goroutine/hub entry not cleaned on close → memory/connection leak.
- Auth not re-checked per message → a closed session's socket still pushes data.
- Fan-out to wrong tenant/room → one user sees another's DMs (cross-tenant leak).
- Optimistic UI update conflicts with server echo → duplicate bubble.

---

## 12. Search (Meilisearch) Bugs `[BE]`

- Index schema missing a field the UI filters on; grouped results (`ADR-017`) return wrong grouping key.
- Document not re-indexed after edit/delete → stale search.
- Tenant filter not applied → cross-tenant discovery of private spaces/users.
- Geo-first ranking (`ADR-005`) ignores user locale/country → irrelevant ordering.
- Typo-tolerance too aggressive → false matches exposing moderated/hidden content.

---

## 13. Payments / Wallet / Credits Bugs `[BE]`

Ledger model (`ADR-024`).
- Double-spend: concurrent debits race past balance check → negative balance or free credit. Use a
  transactional balance lock / `CHECK (balance >= 0)`.
- Refund path doesn't reverse the ledger entry → balance inflation.
- Manual-transfer proof accepted without verifying file/reference → fraud.
- Webhook replay (same provider event id) processed twice → duplicate credit (idempotency).
- Currency mismatch: amount in major units somewhere, minor elsewhere → 100× error.
- Idempotency middleware missing on `POST /payments/intent`, `POST /me/wallet/convert`, or any
  financial endpoint → duplicate charge (Rule 9). Must use `SET NX EX`, not GET-then-SET.

---

## 14. Content / Tiptap / Page-Builder / Media Bugs `[FE/BE]`

- Tiptap output not sanitized server-side → stored XSS (§5.1). Renderer must allowlist tags.
- Page-Builder nested blocks lose data on save (JSON round-trip drift).
- Media upload without validation (`ADR-023`) → malicious file / SSRF / oversized upload DoS. Mime +
  size + dimensions checked server-side; respect `media.max_upload_mb`.
- Render-time affiliate processing (`ADR-009`) injects wrong/no affiliate link, or leaks affiliate id.
- `content_hash` not computed before every `contents` insert/update → duplicate not rejected with
  `409 duplicate_content` (Rule 12).
- `content.edit_window_minutes` / `forum.edit_window_minutes` enforced incorrectly → edit after window
  or edit blocked inside window.

---

## 15. GDPR / NDPR Compliance Bugs `[BE]`

Erasure / anonymize model (`ADR-014`, `ADR-057`).
- Erasure request (`internal/gdpr`) misses a table → PII remains after "delete" (right to be forgotten
  violated). Every PII-bearing store must be covered.
- Anonymization leaves a reversible join → re-identify.
- Soft-delete rows still returned by a query that forgot `deleted_at IS NULL` → leaked erased user.
- Retention cron deletes data still referenced by audit/ledger → FK violation or gap.
- Export omits a store → incomplete portability export.
- Honcho admin visibility vs erasure (`ADR-027`) shows erased data to admin.
- Hard-delete of user content outside the `account:deletion_cleanup` Asynq task (Rule 11).

---

## 16. SSR / Hydration Bugs `[FE]`

Main app is SSR-by-default for public routes (`/`, `/forum/*`, `/blog/*`, `/spaces/*`, `/u/*`,
`/org/*`); CSR-fetched for authenticated views (`/home`, DMs, notifications).
- Hydration mismatch: server renders one `$page.data`, client re-fetches different → text mismatch.
- Skeleton loaders missing on data-driven sections → layout shift.
- `window`/`document`/`localStorage` accessed during SSR → 500 on server.
- Authenticated view fetched client-side but rendered server-side with empty state → SEO leak / blank.
- `adapter-static` admin/control pages importing SSR-only modules → build failure.

---

## 17. Admin / Control Panel SPA Bugs `[FE]`

- Auth guard: `jc_session` cookie check on mount; if missing, redirect must not loop.
- Bits UI key handling in `apps/control` (API keys, extension toggles): key shown in plaintext in DOM,
  copied insecurely, or toggle race leaves inconsistent state across tabs.
- Polling memory leak on `/cache`, `/cron`, `/dlq`, `/db` dashboards (§4.3).
- DataTable spec violations: a list view not using `DataTable` (sort/search/filter/paginate/actions/
  bulk/create) → inconsistent behavior.
- LayerChart analytics using `xNice` on a `BarChart` band scale → scale collapse (band scales don't
  support `d3.nice()`). Use `xPadding` instead.

---

## 18. Rate Limiting / Bot Management Bugs `[BE]`

Layered rate limiting (`ADR-022`), bot/crawler management (`internal/bots`).
- Rate limit keyed by IP only → shared IP users blocked, or abuser rotates IPs freely.
- Authenticated endpoint limited by IP, not by user → can't throttle abusive accounts.
- Bot rule blocks a legit crawler (SEO drop) or allows a bad bot (scraping).
- Rate-limit counter in Valkey without TTL → unbounded growth.

---

## 19. AI Gateway Bugs `[BE/FE]`

Multi-provider AI gateway (`ADR-010`), permission nodes (`ADR-025`).
- `ai.context.*` node not checked → a role uses AI context it shouldn't. Note: **all `ai.context.*`
  nodes default OFF, including for `super_admin`** — every node granted manually (§3-rbac).
- Provider abstraction swallows the real error → user sees generic failure, ops blind.
- Budget (`/ai-budget`) not enforced → runaway cost; no per-tenant cap (`ai.monthly_budget_usd` is an
  alert threshold, not a hard cap).
- Streaming response not cancelled on client disconnect → wasted tokens / goroutine leak.
- pgvector embedding generated with wrong model/dimension → similarity garbage.

---

## 20. Email / Notification Bugs `[BE]`

- Multi-provider email (`ADR-015`) failover not triggered → silent non-delivery.
- Notification batching/dedup (`ADR-035`) collapses unrelated notifications or never flushes a batch →
  missed alerts. Respect `notifications.batch_window_minutes`, `notifications.max_delivery_attempts`.
- WebPush (`webpush-go`) subscription stale → 410 responses not pruned.
- Email template (`apps/admin` `/email-templates`) renders unescaped user data → XSS in email.

---

## 21. Incomplete Features & Stub Handlers `[BE/FE]`

The backend roadmap is 37 phases / 410 tasks and the frontend roadmaps total ~779 tasks. A feature is
"done" only when its route, handler, store, and (if UI) page all exist and behave per the section.

- **Route registered but handler is a stub.** chi route wired to a handler that returns `nil`,
  `200 OK` with empty body, `notImplemented`, or a hardcoded placeholder → UI shows a spinner/empty
  state forever. Grep for `http.Error(..., http.StatusNotImplemented)`, `// TODO`, `panic("stub")`,
  `return nil` at the top of a handler.
- **Wired but no-op mutation.** `PATCH`/`POST` handler parses input and returns success without
  calling the store → data never persists (UI thinks it saved).
- **Missing CRUD leg.** A resource exposes List + Create but no Update/Delete, or no Get-by-id, while
  the section/JEOS route catalog defines all of them. Check `conventions/api-routes-catalog.md` for
  the full expected verb set per resource.
- **Roadmap task accepted without acceptance criteria met.** Per Rule 15, a phase is not complete
  until all ~430 routes return non-404 under correct auth and `gosec` reports zero HIGH/CRITICAL.
- **Frontend page scaffolded, logic missing.** Route file exists but `onMount`/`load` is empty, table
  renders a static placeholder, or "Create" button has no action (violates Rule 16 — must call API).
- **Feature flag off by default but UI always shows the feature.** Flag gating missing on the
  frontend (`feature.dm`, `ai.japabot_enabled`, etc.) → users see controls that 500 on click.
- **Known drift example:** `internal/admin/moderation` wires `/admin/moderation-queue/{id}` and
  `/{id}/appeal`, but frontend moderation/appeals and moderation/reports call `PATCH /admin/appeals/{id}`
  and `/admin/reports/{id}` — endpoints that do not exist. Confirm the route is actually registered
  before trusting the UI.

---

## 22. API Routes Not Conforming to JEOS Business Logic `[BE/FE]`

Every route must match the master `conventions/api-routes-catalog.md` and the owning `sections/*`
file. A route that diverges from JEOS is a bug even when it runs. Cross-check against the **17 Golden
Rules** (`AGENTS.md`):

- **Rule 1 — section is source of truth.** Handler implements behavior contradicting its section
  (e.g., forum "solved" flag logic differs from `sections/08-forum-qa-solved.md`).
- **Rule 2 — never hand-edit `internal/db/`.** Generated sqlc output edited directly → overwritten on
  next `make sqlc-gen`, causing silent drift. Fix the `.sql` in `db/queries/`.
- **Rule 3 — no hardcoded config.** Magic numbers / URLs / limits in code instead of `JC_*` env or
  `system_settings` via `GetSetting(ctx, key, default)`. Wrong: `if memberCount > 5000`. Right:
  `config.GetIntSetting(ctx, "feed.fanout_threshold", 5000)`.
- **Rule 4 — all Valkey keys use `jc:` prefix + catalog entry.** A key pattern not in
  `valkey-keys-catalog.md` (34 patterns) is undocumented → add to catalog first, then code.
- **Rule 5 — standard error envelope.** Error uses `{ "error": { "code": "snake_case", "message" } }`,
  not a plain string; success uses `{ "data": ... }`; lists are offset-paginated via
  `?page_size=N&page_offset=N` (no cursor).
- **Rule 6 — permission cache invalidation.** Any `users.user_group_id` change, `user_role_assignments`
  insert/delete, `space_moderators` insert/delete, `role_permissions` change, or `user_bans` insert
  must call `InvalidatePermCache`. Forgetting it → stale permissions for up to 5 min.
- **Rule 7 — Asynq task name in catalog.** Unknown task name silently drops (see §10).
- **Rule 8 — fan-out is fire-and-forget.** Handler waits for `content:fan_out_to_feeds` to finish.
- **Rule 9 — idempotency middleware on financial endpoints** using `SET NX EX` (see §13).
- **Rule 10 — impersonation writes to `audit_logs`.** Every start/terminate/end must insert a row; a
  failed Valkey write must abort (never impersonate unlogged).
- **Rule 11 — soft delete only for user content in V1.** `DELETE FROM contents/comments/media` in app
  code is forbidden; use `deleted_at = now()`. Only `account:deletion_cleanup` may hard-delete.
- **Rule 12 — `content_hash` computed before every save** → unique index rejects duplicates with
  `409 duplicate_content`.
- **Rule 13 — circular FK migration ordering** (users ↔ spaces) must follow the stub pattern.
- **Rule 14 — multi-section tasks read all referenced sections.** A feature implemented against one
  section but contradicting another (e.g., 05 + 14 + 32) is internally inconsistent.
- **Rule 15 — all ~430 routes return non-404 under correct auth.** A 404 under valid RBAC is an
  unimplemented/incompletely-wired route.
- **Rule 16 — frontend consumes the API only.** No hardcoded data arrays, mock data, static option
  lists (countries, currencies, languages), or local-only state mutations for server data (see §27).
- **Rule 17 — no AI authorship attribution.** No "Generated by AI", "Co-Authored-By: Claude", or agent
  trailers in commits, comments, or docs.

---

## 23. Incomplete Form Data & Field Validation `[BOTH]`

Forms (custom-field-backed or bespoke) must validate the full contract the section defines.
- **Required field accepted empty.** `is_required` custom field or section-required field (e.g.,
  `content.min_length` = 10, `forum.edit_window_minutes`) not enforced server-side → blank/invalid
  rows persist.
- **Multi-step form skips steps.** `is_multi_step` form submitted without all `form_steps` completed →
  partial `form_submissions.field_values` jsonb.
- **Anonymous vs authenticated mismatch.** `form_submissions.user_id` nullable for anonymous but the
  handler requires auth → anonymous feedback/report forms 403.
- **Rate limit not applied.** `POST /api/v1/forms/:slug/submit` must be limited to 10 submissions per
  user per hour; missing limiter → form-spam.
- **Field type not validated on ingest.** `url`/`email`/`phone`/`color` custom fields stored without
  format validation → bad data downstream (see §24).
- **Trigger not fired.** `form_submission_triggers` (`assign_user_group` / `send_email` / `send_webhook`
  / `create_ticket` / `notify_admin`) not executed async after submit → expected side effect missing.
- **Edit window ignored.** Content/comment/forum edited after `content.edit_window_minutes` /
  `comments.edit_window_minutes` / `forum.edit_window_minutes` → stale edits or blocked-in-window bug.

---

## 24. Incomplete / Incorrect Fields & Data Types `[BE]`

Every table column has a defined type and constraint in its section; mismatches corrupt data.
- **Wrong Go type for a column.** `numeric` mapped to `int` (truncation); `timestamptz` stored as
  string; `jsonb` scanned into a struct without `pqtype`/codec → unmarshal error or lost data.
- **Nullable column scanned into non-pointer** → NPE (§6.3).
- **Truncation.** `text` field (max 255) stores longer string → DB error or silent trim.
- **`value_*` column mismatch in `custom_field_values`.** `value_text`/`value_number`/`value_bool`/
  `value_date`/`value_json` written to the wrong column for the `field_type` → queries/filters return
  garbage. `reference` fields must store `value_json: {id, entity_type, label}` and be resolved at
  render time, never raw-joined.
- **Enum/check constraint drift.** A `status`/`purpose_type` value used in code that isn't in the
  column's CHECK/enum → insert rejected.
- **Timestamps not ISO8601 UTC.** Frontend sends local time; backend stores without `time.Time` UTC
  normalization → off-by-timezone bugs in feeds/scheduling.
- **Polymorphic discriminator missing** on union responses (§3.3).

---

## 25. Custom Fields Architecture Bugs `[BE/FE]`

Universal custom-field system (`sections/06-custom-field-builder-form-builder.md`, `ADR-030`): one
`custom_field_groups` + `custom_fields` model underlies Space Types, Content Types, User/Org Profiles,
Forms, Tags, Tickets.
- **Field defined but not RBAC-scoped.** `custom_field_permissions` (can_view/can_edit per UserGroup)
  missing → a field visible/editable by a group that shouldn't see it (PII leak).
- **`field_type` not in the allowed set** (text, textarea, rich_text, url, email, phone, color,
  number, currency, percentage, date, datetime, time, duration, boolean, select, multiselect,
  image_single, image_multi, media, platform_embed, reference, rating) → renderer can't display it.
- **`options` jsonb shape wrong.** select/multiselect expect `[{value,label}]`; number expects
  `{min,max,is_decimal}`; currency expects `{currency_code}`; image_* expect `{max_files,max_size_mb,
  aspect_ratio}`; reference expects `{ref_entity_class}`.
- **`is_required` + `show_in_frontend` + `filterable`/`sortable`/`searchable` inconsistency.** A
  `filterable` field not indexed (`idx_cfv_filterable`); a `searchable` field not pushed to Meilisearch.
- **System field edited/deleted.** `is_system` fields (user_profile_fields, blog_post_fields, etc.)
  mutated by admin → breaks seed-dependent UI.
- **`platform_embed` resolver missing a platform** in `embed_platforms` → `ErrUnsupportedPlatform`
  instead of embed. Only youtube/github/twitter/x/vimeo/spotify/figma/codepen/maps supported.
- **Assignment `entity_class` typo** (`content_type:{slug}` / `space_type:{id}` / `user_profile` /
  `organization` / `form:{id}` / `tag` / `ticket_type:{id}`) → field group never attached, form renders
  without fields.
- **`JapaStatus` options drift.** Admin changed the `select` options but old user values now orphaned.

---

## 26. RBAC CRUD Completeness & Backend Wiring `[BE/FE]`

Dynamic three-tier RBAC (`sections/03-rbac.md`, `ADR-003`): User Groups → User Roles → Permissions,
fully admin-configurable; **no hardcoded role checks anywhere**.
- **Route enforces a node that doesn't exist in `permissions`.** Routes enforce ~249 nodes using
  `_own`/`_any` scoping (`blog.edit_own`, `media.delete_any`, `content.reactions.add`); the original
  seed defined only 75, so 174 enforced nodes were missing — meaning **no role, including `super_admin`,
  could hold them**. Verify every `hasPermission(ctx, userID, node)` call's node is seeded (migration
  `000109_rbac_permission_reconciliation` is the canonical fix; `members` should hold exactly 45
  member-level nodes).
- **`members` grant references keys the route never checks** (or vice-versa) → members see 403 on a
  feature they should use, or can't use one they should.
- **Node naming drift.** Enforced node uses dot-scoping (`content.edit.own`) but convention is
  underscore (`content.edit_own`) → permission never matches a seeded key.
- **Runtime-resolved node not handled.** `spaces.moderate.assigned` is resolved by querying
  `space_moderators`, not from the permissions table; a handler that looks it up as a static grant
  always denies. `spaces.moderate.any` is a static grant (super_admin + optional moderation group).
- **`spaces.moderator.assign` enforced only at handler** (space creator) — a missing check lets a
  non-creator assign moderators.
- **Quantity gate ignored.** `spaces.create` must respect `user_groups.max_spaces_creatable`
  (0 = none, -1 = unlimited, N = capped).
- **Permission cache invalidation missing** after the five mutation types (Rule 6) → up to 5-min stale
  capability. For role-wide permission changes, either rely on TTL or `SCAN jc:perm:*` + batch DEL for
  security-critical removals, and log the decision.
- **Group membership not points-automated** (Section 03 amendment): no job/threshold may move a user
  between groups based on Points (ADR-028). A hunter should flag any code that promotes/demotes by
  points total.
- **Frontend calls an endpoint the backend never registers** (see §21). The admin sidebar nav map in
  `apps/admin/src/routes/+layout.svelte` is the UI contract; backend `server.go` is the wire. They must
  agree node-for-node.
- **`api.keys.*` nodes.** `api.keys.self_manage` (verified members, on by default), `api.keys.view.all`,
  `api.keys.manage`, `api.keys.analytics.view` — missing gate → any user manages/revokes others' keys.

---

## 27. Frontend Rule 16 (API-Only) Compliance `[FE]`

Per Rule 16, every SvelteKit surface must consume the backend; no hardcoded/mock/static data.
- **Hardcoded option lists.** Countries, currencies, languages, space types, tags defined inline
  instead of fetched (`GET /api/v1/spaces?level=country`, feature flags, etc.).
- **Mock/placeholder user data** in production components (`const user = {name:'Test',credits:100}`).
- **Static feature check.** `const hasDM = true` instead of reading `feature-flags` / `GET /me`.
- **Local-only state mutation for server data.** `items = items.filter(...)` before confirming server
  success → UI diverges from truth after a failed delete. Use SvelteKit `invalidate()` after the API
  confirms.
- **`+page.server.ts` `load()` returns hardcoded data** instead of calling the API server-side.
- **`+page.ts` `load()` returns static data** on the client instead of fetching.
- **Form action writes to local state** instead of calling the backend route.
- **Store populated with API data not invalidated on mutation** (manually patched instead).

---

## 28. Known JEOS Conformance Gaps to Re-verify

These were flagged in prior audits and are high-value hunting targets:
- 174 RBAC nodes missing from seed (super_admin/members could not hold them) — verify reconciliation
  migration applied and nodes now resolvable.
- Flat Valkey keys `jc:llms_txt`, `jc:llms_full_txt`, `jc:robots_txt` not tenant-scoped.
- Moderation endpoints called by frontend (`/admin/appeals/{id}`, `/admin/reports/{id}`) not wired in
  backend (`internal/admin/moderation` only wires `/admin/moderation-queue/{id}` and `/{id}/appeal`).
- `permissions` table schema drift: code uses `id uuid PK` + `key text UNIQUE` and
  `role_permissions.permission_id → permissions.id`, not the illustrative `node text PK` in §03.

---

## Bug Hunter's Checklists

### A. JEOS Conformance (business-logic) checklist
1. Does every route match `conventions/api-routes-catalog.md` and its owning `sections/*` file?
2. Is every ~430 route reachable (non-404) under correct auth (Rule 15)?
3. Does every `hasPermission()` node exist in the `permissions` seed, using `_own`/`_any` scoping?
4. After every group/role/mod/ban mutation, is `InvalidatePermCache` called (Rule 6)?
5. Is every Valkey key `jc:`-prefixed, tenant-scoped, and cataloged (Rules 4, §4.2)?
6. Are all config values from `JC_*`/`system_settings`, never hardcoded (Rule 3)?
7. Does every financial endpoint use `SET NX EX` idempotency (Rule 9)?
8. Is all user content soft-deleted; only `account:deletion_cleanup` hard-deletes (Rule 11)?
9. Is `content_hash` computed before every `contents` save (Rule 12)?
10. Is every Asynq task name in the Section 11 / asynq-tasks catalog (Rule 7)?

### B. Feature-completeness checklist
1. For each resource: are List/Get/Create/Update/Delete all implemented and non-stub?
2. Does each handler call the store (no no-op success)?
3. Does each frontend page call the API (no hardcoded/mock data — Rule 16)?
4. Is every bespoke/custom form field validated (required, type, edit-window, rate limit)?
5. Is each custom field RBAC-scoped, correctly typed, and assignment `entity_class` correct?
6. Does every GDPR erasure path cover all PII tables and exclude erased rows via `deleted_at`?
7. Are all user-facing strings wrapped in `$t()` with `en.json` + `fr.json` keys?
8. Are Svelte 4 patterns gone (`export let`, `$:`, `on:`, `createEventDispatcher`)?
9. Is Tiptap/Page-Builder HTML sanitized server-side before storage/render?
10. Do wallet/credits ledgers use integer minor units with a non-negative balance lock?
