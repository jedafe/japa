# Agent Data API Phase 3 — Design

**Date:** 2026-09-24
**Status:** Draft (awaiting review)
**Refs:** docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md,
docs/superpowers/specs/2026-09-24-browser-extension-design.md,
japaconnect-engineering/sections/47-public-api-developer-portal.md (API
Permission Scopes), sections/12-webhooks.md, sections/14-content-type-system.md,
sections/81-scam-alert-board.md

## Context & Motivation

Phase 2 shipped the read surfaces (feeds, semantic search, scam alerts) and the
browser extension shipped as a read-only thin client on top of them. The
explicitly deferred Phase 3 scope was:

- Bookmarks surface
- Write APIs via API keys
- Webhooks / push surfaces for agents

Two concrete consumers now exist: the extension's "save to account" action
(bookmark write), and agents that want event push rather than digest polling
(webhooks). Phase 3 turns the API from read-only into a read-write surface —
the point where abuse cost becomes real, so the design is deliberately narrow.

None of this is greenfield:

- Bookmarks have full session CRUD (`POST/DELETE /content/{id}/bookmark`,
  `PATCH …/folder`, `GET /me/bookmarks`, folder service) behind
  `feature.bookmarks` + `bookmarks.create` RBAC.
- `internal/webhooks` is a complete delivery system — HMAC-SHA256 signed
  envelopes, SSRF-rejecting dialer, challenge/response endpoint verification,
  `webhook_delivery` critical-tier retry worker. Admin-managed today (§12);
  org self-service is deferred to v2, so Phase 3 proposes **key-scoped personal
  subscriptions**, a different and smaller surface.
- §47 already catalogs the write scopes (`api.write.content`,
  `api.write.comments`, `api.write.me`) and mandates that write scopes are
  **admin-approved per key**, not self-serve mintable.
- `mw.IdempotencyMiddleware` exists (payments use it) for safe POST retries.

## Goals

- `GET /public/me/bookmarks` — key owner's bookmarks, `api.read.me`.
- `PUT`/`DELETE /public/me/bookmarks/{content_id}` — `api.write.bookmarks`
  (new catalog scope; bookmark mutation is neither profile edit nor content
  write, so it does not belong under `api.write.me` or `api.write.content`).
- `POST /public/content` — create **draft** content via `api.write.content`.
  API keys can never publish directly: `status` is forced to `draft`,
  publish stays behind the session UI / review flow.
- `POST/GET/DELETE /public/me/webhooks` — key-scoped webhook subscriptions
  using the existing §12 delivery infra. Event set limited to what the key
  owner may observe (see Webhooks below).
- All writes: `Idempotency-Key` required on POST/PUT (Rule 9 pattern,
  `SET NX EX`), standard error envelope, per-key rate limiting unchanged.

## Non-Goals

- **Comment writes** (`api.write.comments` stays unused) — spam economics of
  anonymous-keyed commenting need their own moderation story; deferred.
- **Content publish/update/delete** via API key — draft creation only.
  Update/delete have edit-window and audit semantics that don't map cleanly
  to key auth.
- **Org/platform webhook subscriptions** — §12 defers org self-service to v2;
  Phase 3 is personal key-scoped subscriptions only.
- **Payment/profile writes** — `api.write.me` stays cataloged but unimplemented.
- **Scam report submission** — tempting, but report quality needs the session
  moderation flow; deferred until an abuse model exists.

## Bookmarks

Routes (catalog additions):

```
GET    /public/me/bookmarks                    [api.read.me]
PUT    /public/me/bookmarks/{content_id}       [api.write.bookmarks]  body: { folder_id? }
DELETE /public/me/bookmarks/{content_id}       [api.write.bookmarks]
```

- `PUT` is idempotent by nature (bookmark exists = success) but still accepts
  `Idempotency-Key` for consistency with the write surface.
- `folder_id` optional; resolves against `bookmark_folders` owned by the key
  owner — 404 on foreign/missing folder, never silently cross-assign.
- Read shape mirrors the session `ListMyBookmarks` DTO minus internal fields.
- Honors `feature.bookmarks` — gated off → 404 like the session routes.
- The content target must be visible to the key owner (reuse the feed
  visibility rules — public + member spaces the owner belongs to). Bookmarking
  a non-visible ID returns 404, not 403 — don't leak existence.

## Content drafts

```
POST   /public/content    [api.write.content, admin-approved]   body: CreateContentInput subset
```

- Forced `status='draft'` — the `status` field is not accepted from the caller.
- Accepts `title`, `body` (TipTap JSON or markdown — same dual-path as session
  create), `content_type` (restricted allowlist: `blog_post`, `guide`,
  `story`, `discussion` — no `job_listing`/`event`/comparison types that carry
  structured side-effects), `space_id` (must be a space the key owner can post
  to — reuse `UpdateContentSpaces` permission checks), `tags`,
  `field_values`, journey metadata.
- `content.Service.CreateContent` handles `body_html` rendering, `content_hash`
  dedupe (Rule 12 — 409 `duplicate_content` surfaces unchanged), and the
  autosave/hash edge cases.
- Admin-approval flow is already the §47 convention: write scopes are granted
  per-key by an admin, so no new approval machinery is needed.
- RBAC parity: the key owner must hold `content.create`-equivalent permission;
  the service's existing checks run unchanged with `userID = key owner`.

## Webhooks (key-scoped subscriptions)

```
POST   /public/me/webhooks          [api.write.me]   { url, event_types[] }
GET    /public/me/webhooks          [api.read.me]
DELETE /public/me/webhooks/{id}     [api.write.me]
GET    /public/me/webhooks/{id}/deliveries  [api.read.me]  — last N deliveries
```

- Reuse `internal/webhooks` wholesale: envelope format, HMAC signing with a
  per-subscription secret (returned once at create), SSRF-protected dialer,
  `webhook_delivery` retry worker. The `webhooks` table needs an owner
  discriminator — `user_id uuid NULL` column (migration), distinct from the
  admin-managed platform rows.
- **Challenge/response verification** (§12) is mandatory before activation —
  unverified subscriptions never receive deliveries.
- Event catalog for personal subscriptions (Phase 3 subset):
  `content.published` — filtered to content visible to the key owner
  (public + member spaces they belong to). The delivery worker gains a
  per-subscription visibility check against the event's content row.
- `payment.*`, `claim.*`, `organization.verified` stay admin-only — personal
  subscriptions must not see platform events they can't contextualize.
- Max subscriptions per key: `api.webhooks.max_per_key` system setting,
  default 5 (catalog addition).
- Delivery auth: existing `X-Webhook-Signature` HMAC header; the subscriber
  verifies with the secret returned at create.

## AuthN/Z, rate limits, abuse

- Reads use existing read scopes; writes use the `api.write.*` scopes above.
- Write scopes are admin-approved per §47 — the mint UI must not offer them
  self-serve (they're already absent from the self-serve scope picker).
- Every write route runs `mw.IdempotencyMiddleware` — `Idempotency-Key`
  header required on POST/PUT, `SET NX EX` semantics, 409 on in-flight dupe.
- Keyed writes consume the key's existing sliding-window budget; no separate
  write quota in V1.
- `api.write.*` keys get `rate_limit_tier` capped at `standard` unless admin
  explicitly elevates — high-volume write bots are not a launch goal.

## Data / migrations

- `webhooks.user_id uuid NULL REFERENCES users(id)` + index — distinguishes
  personal subscriptions from admin platform webhooks. `NULL` = admin-managed.
- `bookmark` writes reuse the existing `bookmarks` table — no schema change.
- New system setting: `api.webhooks.max_per_key` (default 5).

## Testing

- Unit: bookmark DTO mapping, folder ownership check, draft-forcing
  (status param ignored/rejected), webhook event visibility filter.
- Integration (`JC_DATABASE_URL`): full bookmark round-trip via key,
  draft create → appears in `GET /me/content` drafts, webhook register →
  challenge → activate → `content.published` fires to httptest receiver with
  valid HMAC.
- Gate tests: `api.read.*` key cannot hit write routes; write-scope key
  without approval cannot mint (mint path unchanged).
- Adversarial: bookmark of non-visible content → 404; webhook URL to
  private IP range → rejected by SSRF dialer (existing test pattern).

## Open questions

1. Should `PUT /public/me/bookmarks/{id}` support folder creation inline, or
   keep folders session-only in V1? (Proposed: session-only — folder CRUD via
   API is scope creep.)
2. Draft body format: accept markdown only, or TipTap JSON too? (Proposed:
   both — `CreateContent` already handles both; no reason to restrict.)
3. Webhook `content.published` granularity: all visible content, or filter by
   subscription `event_filter` (space/tag)? (Proposed: all-visible in V1 —
   filters are additive later.)
4. Should key-scoped webhooks show up in `/admin/webhooks` for ops
   visibility? (Proposed: yes, read-only there — no new admin surface.)
5. Extension "save" button: PUT bookmark on card tap — needs the extension
   key to optionally carry `api.write.bookmarks`, which conflicts with the
   extension-token narrow-scope model. Proposed: extension tokens stay
   read-only; the save action deep-links to the site. Revisit post-launch.
