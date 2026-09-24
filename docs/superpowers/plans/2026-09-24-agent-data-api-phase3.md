# Phase 3 Implementation Plan — bookmarks, drafts, key-scoped webhooks

**Spec:** docs/superpowers/specs/2026-09-24-agent-data-api-phase3-design.md
**Date:** 2026-09-24
**Branch:** main (per AGENTS.md)

## Verified corrections to the spec (pre-plan review)

1. **`api.write.*` scopes ARE self-serve today** — `scopes.ScopeCatalog`
   returns them to `GET /developer/scopes`, and `CreateAPIKey` validates
   nothing (a user can mint `*` right now). §47's "admin-approved per key"
   rule is unenforced. The plan adds a Task that closes this — without it,
   shipping write endpoints would let anyone self-mint write access.
2. **Table is `webhook_endpoints`, not `webhooks`** — §12's schema block is
   outdated (migration 000361 conformance). Columns: url, secret,
   event_types, status, is_active, verified_at, `created_by → users(id)
   ON DELETE SET NULL`. `tenant_id` was dropped by 000361 — no tenant work
   needed. Personal subscriptions get a `scope` column ('platform'|'user'),
   owner = `created_by`.
3. **Event emission is live** — `content.Service.emitWebhook` fires
   `content.created/published/updated/deleted` via
   `webhooks.Service.EmitEvent`, which fans out to all `is_active AND
   verified_at IS NOT NULL` endpoints matching `event_types`. Payload:
   `{content_id, slug, title, actor_id}` — **no visibility fields**, so the
   per-user check happens at delivery time against the content row.
4. **`visibility='members'` means any authenticated user** — the SQL
   predicate is `visibility='members' AND @can_see_members`, not
   space-membership. The per-subscriber filter is: `status='published'` AND
   (`visibility='public'` OR (`'members'` AND subscriber is a user)). Space
   privacy is a separate predicate — replicate the feed queries' space join.
5. **`Idempotency-Key` scope**: `mw.IdempotencyMiddleware` exists and is used
   for payments only. Phase 3 applies it to `POST /public/content` and
   `POST /public/me/webhooks`; `PUT/DELETE` bookmarks are naturally
   idempotent — the middleware accepts the header but won't 409 without one.
   Check the middleware's required-vs-optional behavior at implementation.
6. **Bookmark handlers** are `contentHandlers.AddBookmark/RemoveBookmark`
   (`POST/DELETE /content/{id}/bookmark`, `feature.bookmarks`-gated,
   `bookmarks.create` RBAC) and `usersHandlers.ListMyBookmarks`
   (`GET /me/bookmarks`). The public API wraps `content.Service`
   (`AddBookmark/RemoveBookmark/ListBookmarks` in store.go:2194-2222) —
   same seam as Phase 2's readers.
7. **`webhook_deliveries`** already exists keyed by `endpoint_id` —
   `GET /public/me/webhooks/{id}/deliveries` is a list query, no new tables.
8. **`{content_id}` in the bookmark route** — chi routes already use `{id}`
   for `/content/{id}/bookmark`; public route uses `{content_id}` to avoid
   confusion with `/public/content/{id}` GET (param names are scoped per
   route, so either works — keep `{content_id}` for clarity).

## Task 1 — JEOS docs-first (`.jeos` submodule + parent bump)

- `conventions/api-routes-catalog.md`: add
  `GET /public/me/bookmarks` [api.read.me],
  `PUT /public/me/bookmarks/{content_id}` [api.write.bookmarks],
  `DELETE /public/me/bookmarks/{content_id}` [api.write.bookmarks],
  `POST /public/content` [api.write.content],
  `POST /public/me/webhooks` [api.write.me],
  `GET /public/me/webhooks` [api.read.me],
  `DELETE /public/me/webhooks/{id}` [api.write.me],
  `GET /public/me/webhooks/{id}/deliveries` [api.read.me].
- `conventions/system-settings-catalog.md`: `api.webhooks.max_per_key`
  (int, default 5).
- §47 API Permission Scopes: add `api.write.bookmarks` leaf; document that
  `api.write.*` scopes are admin-only in the self-serve catalog and rejected
  by `CreateAPIKey` for non-admins (new enforcement, Task 6).
- §12 webhooks: fix `webhooks` → `webhook_endpoints`; add `scope` column
  ('platform' default, 'user' for key-scoped personal subscriptions);
  document per-subscriber visibility filtering for `content.*` events and
  the Phase 3 user-subscription routes.

## Task 2 — Migration + sqlc

- `migrations/000732_webhook_endpoint_user_scope.up.sql`:
  `ALTER TABLE webhook_endpoints ADD COLUMN scope text NOT NULL DEFAULT
  'platform' CHECK (scope IN ('platform','user'));
   CREATE INDEX idx_webhook_endpoints_user ON webhook_endpoints(scope,
   created_by) WHERE scope='user';` (+ `.down.sql` DROP COLUMN).
- `internal/db/queries/webhooks.sql`: add `CreateUserWebhookEndpoint`
  (scope='user', created_by=@user_id, is_active=false, verified_at=NULL —
  pending challenge), `ListUserWebhookEndpoints`,
  `DeleteUserWebhookEndpoint` (WHERE scope='user' AND created_by=@user_id),
  `ListWebhookDeliveriesByEndpoint` if absent.
- `contents.sql`: `IsContentVisibleToUser :one` — id,status,visibility +
  space visibility for a single content ID against a viewer (published +
  public, or members for authenticated; private space → invisible unless
  member). Reuse the exact predicates from the feed queries.
- `make sqlc-gen`.

## Task 3 — Bookmarks surface

- `internal/publicapi/bookmarks.go`: `ListMyBookmarks` (api.read.me) →
  `content.Service.ListBookmarks` DTO-mapped; `PutBookmark`/`DeleteBookmark`
  (api.write.bookmarks) → `AddBookmark`/`RemoveBookmark` with
  `userID = key owner`.
- Visibility: before Add, load content via the Task-2 visibility check —
  invisible/absent → `404 content_not_found` (no existence leak).
- `folder_id` optional; foreign/non-owned folder → `404 folder_not_found`.
- `feature.bookmarks` gate parity: disabled → 404 (mirror session routes).
- Handlers in `internal/publicapi/handlers_phase3.go`; routes under the
  existing `/public` group in `server.go`.

## Task 4 — Content draft endpoint

- `POST /public/content` → `RequireScope("api.write.content")` +
  `mw.IdempotencyMiddleware`.
- Body: `{title, body, body_html?, content_type, space_id?, tags?,
  field_values?, journey metadata}`; `status` forced `'draft'` — a caller-
  supplied `status` is rejected with `400 status_forbidden` (explicit beats
  silent ignore).
- `content_type` allowlist: `blog_post|guide|story|discussion`; others →
  `400 unsupported_content_type`.
- `space_id` — reuse the content service's existing space-posting checks;
  missing → personal space.
- Calls `content.Service.CreateContent` with `userID = key owner` —
  body_html render, content_hash dedupe (409 `duplicate_content`), RBAC all
  inherit unchanged.
- Response: `201` with `{id, slug, status: "draft"}` — no full DTO.

## Task 5 — Key-scoped webhook subscriptions

- `internal/publicapi/webhooks.go` + handlers: register (URL + event_types
  allowlist [`content.published` only in V1] → create endpoint row
  scope='user' + return secret once + fire verification challenge),
  list, delete, list deliveries. `max_per_key` setting enforced.
- Reuse `webhooks.Service` verification flow (challenge → `verified_at`)
  — expose a `CreateUserEndpoint`/`VerifyChallenge` seam rather than
  duplicating the HMAC/challenge logic.
- `EmitEvent` change: `scope='user'` endpoints get deliveries only when the
  event is `content.published` AND `IsContentVisibleToUser(content_id,
  created_by)` passes — check at fan-out (payload carries content_id).
- Delivery worker unchanged (endpoint-scoped already).
- SSRF dialer + secret generation + HMAC signing: reused untouched.

## Task 6 — Write-scope admin gating (closes the spec gap)

- `scopes.ScopeOption` gains `AdminOnly bool`; `api.write.*` + `*` marked
  admin-only.
- `GET /developer/scopes` filters admin-only entries for non-admin callers
  (admin check = `user_group` admin/super_admin or RBAC `admin.*` — use the
  same check the admin routes use).
- `CreateAPIKey` rejects admin-only scopes for non-admin owners:
  `403 scope_requires_admin`. Wildcard `*`/`api.write.*` likewise.
- `CreateExtensionToken` unaffected (fixed narrow scope set).

## Task 7 — OpenAPI + verify

- `internal/publicapi/openapi.go`: bookmark/draft/webhook paths + schemas;
  note Idempotency-Key header on writes.
- Tests: bookmark round-trip, invisible-content 404, draft forcing,
  content_type allowlist, scope denial (`api.read.*` key → 403 on writes),
  non-admin `api.write.*` mint → 403, webhook register→verify→deliver via
  httptest receiver + visibility filter (private content doesn't fire for
  outsider-owned subscription).
- `go build ./...`, vet, package tests; `make sqlc-gen` committed output.
- Live smoke on :8081: mint write key (as admin path), bookmark PUT/DELETE,
  draft POST, webhook register against a local receiver.

## Order & dependencies

Task 1 → Task 2 (schema) → Tasks 3/4/5 parallel-able → Task 6 (independent,
do early — it's a live gap) → Task 7.
