# Route Catalog ↔ Code Reconciliation — Implementation Plan

**Date:** 2026-07-09
**Scope:** Complete P1 (catalog documentation) + remaining P2/P3 renames + route guard test
**Current diff:** MATCH=497, CATALOG_ONLY=224, CODE_ONLY=131
**Target:** CODE_ONLY → 0 (or near-zero for intentional infra); CATALOG_ONLY shrinks by ~131

---

## Phase 1: Route Guard Test (Task 1.1 from plan)

### Task 1.1.1: Create `internal/server/routes_test.go`

The test file does not exist. Create it with the no-duplicate-routes test from the plan spec (lines 61–88 of the plan file). This is a compile-time scaffold that will be fleshed out in P6 when a test DB is available.

**Files:**
- Create `internal/server/routes_test.go`

**Steps:**
1. Write the `TestNoDuplicateRoutes` function matching the spec
2. `go build ./internal/server/` — confirm PASS
3. Commit: `test(server): seed route guard scaffold`

---

## Phase 2: Complete P1 — Document CODE_ONLY routes in catalog

These 131 code-only routes need documentation in `.jeos/conventions/api-routes-catalog.md`. Group them into sub-commits for reviewability. All changes are doc-only to the catalog file.

### Task 2.1: Forum extras → catalog (§126–141)

Add missing forum routes to the existing Forum section:

```
DELETE /forum/:id/vote
DELETE /forum/:id/solve
GET    /forum/:id/replies
POST   /forum/:id/replies
PATCH  /forum/replies/:id
DELETE /forum/replies/:id
GET    /forum/polls/:id
POST   /forum/polls
POST   /forum/polls/:id/vote
DELETE /forum/polls/:id/vote
GET    /forum/tags
GET    /forum/tags/:slug
POST   /forum/tags
PATCH  /forum/tags/:id
DELETE /forum/tags/:id
GET    /forum/bookmarks
POST   /forum/bookmarks
DELETE /forum/bookmarks/:id
POST   /forum/:id/tags/:tag_id   (code has this form)
```

**Commit:** `docs(catalog): document forum extras present in code`

### Task 2.2: Comparison superset extras → catalog (§353–364)

The code has a richer superset of comparison routes. Add to the Comparisons section:

```
GET    /comparisons/:id
GET    /comparisons/:id/matrix
POST   /comparisons/:id/items/reorder
GET    /comparisons/types
GET    /comparisons/types/:slug
POST   /comparisons/types
PATCH  /comparisons/types/:id
DELETE /comparisons/types/:id
PUT    /comparisons/types/:id/access
GET    /comparisons/types/:id/access
POST   /comparisons/:id/dimensions
PATCH  /comparisons/dimensions/:id
DELETE /comparisons/dimensions/:id
PUT    /comparisons/dimensions/:id/values
DELETE /comparisons/:id
DELETE /comparisons/:id/items/:id
DELETE /comparisons/dimensions/:id
```

**Commit:** `docs(catalog): document comparison superset routes`

### Task 2.3: Professional profiles superset extras → catalog (§253–267)

Add under the existing Profiles section:

```
GET    /profiles/:id/author-context
GET    /profiles/:id/documents
POST   /profiles/:id/documents
PATCH  /profiles/documents/:id
DELETE /profiles/documents/:id
GET    /profiles/:id/relocations
POST   /profiles/:id/relocations
PATCH  /profiles/relocations/:id
DELETE /profiles/relocations/:id
GET    /profiles/:id/verifiers
POST   /profiles/:id/verifiers
DELETE /profiles/verifiers/:id
POST   /profiles/invitations/:token/accept
GET    /profiles/types
GET    /profiles/types/:slug
POST   /profiles/types
```

**Commit:** `docs(catalog): document professional profile superset routes`

### Task 2.4: User `/me` extras → catalog (§24–96)

Add missing `/me/*` routes:

```
GET/PATCH  /me/preferences             (dark_mode + language — distinct from notification-preferences)
GET    /me/wallet/ledger
GET    /me/payments/:id
GET    /me/api-keys/:id
PATCH  /me/api-keys/:id
GET    /me/api-keys/:id/ip-allowlist
POST   /me/api-keys/:id/ip-allowlist
DELETE /me/api-keys/:id/ip-allowlist/:ip_id
POST   /me/api-keys/:id/rotate
GET    /me/totp/status                 (already documented, verify)
```

**Commit:** `docs(catalog): document /me extras present in code`

### Task 2.5: Admin extras → catalog (§521–762)

Add the large block of missing admin routes. These are all code-only and need documenting:

```
-- Moderation (rename target for /admin/moderation → /admin/moderation-queue)
GET    /admin/moderation-queue
POST   /admin/moderation-queue/:id/assign
POST   /admin/moderation-queue/:id/appeal
POST   /admin/moderation-queue/bulk

-- Broadcast (rename target)
GET    /admin/broadcasts
POST   /admin/broadcasts
POST   /admin/broadcasts/:id/send
DELETE /admin/broadcasts/:id

-- Ads campaign actions
POST   /admin/ads/campaigns/:id/activate
POST   /admin/ads/campaigns/:id/pause
POST   /admin/ads/campaigns/:id/end
DELETE /admin/ads/campaigns/:id

-- Ads boosts
GET    /admin/ads/boosts
GET    /admin/ads/boosts/active
POST   /admin/ads/boosts
POST   /admin/ads/boosts/:id/end

-- i18n manual upsert extras
POST   /admin/i18n/strings            (manual upsert)
POST   /admin/i18n/strings/batch
DELETE /admin/i18n/strings/:id

-- Learn admin extras
POST   /admin/learn/tracks/:id/publish
POST   /admin/learn/tracks/:id/modules
POST   /admin/learn/modules/:id/lessons
PATCH  /admin/learn/modules/:id
DELETE /admin/learn/modules/:id
PATCH  /admin/learn/lessons/:id
DELETE /admin/learn/lessons/:id
GET    /admin/learn/tracks/:id/enrollments

-- Wallet admin extras
GET    /admin/wallet/settings
PUT    /admin/wallet/settings
POST   /admin/wallet/adjust
GET    /admin/wallet/user/:id/balance
PATCH  /admin/wallet/rules/:id/toggle

-- Discount codes extras
PATCH  /admin/discount-codes/:id
GET    /admin/discount-codes/:id/usage

-- Tags admin extras
POST   /admin/tags/:id/moderators
DELETE /admin/tags/:id/moderators/:user_id

-- Notification types
GET    /admin/notification-types
POST   /admin/notification-types
PATCH  /admin/notification-types/:id
DELETE /admin/notification-types/:id

-- Reaction types admin
GET    /admin/reaction-types
POST   /admin/reaction-types
PATCH  /admin/reaction-types/:key
DELETE /admin/reaction-types/:key

-- Universities admin
GET    /admin/universities
POST   /admin/universities
PATCH  /admin/universities/:id
DELETE /admin/universities/:id
POST   /admin/universities/:id/verify

-- Comparison types admin
GET    /admin/comparison-types
POST   /admin/comparison-types
PATCH  /admin/comparison-types/:id
DELETE /admin/comparison-types/:id

-- Reviews admin
PATCH  /admin/reviews/:id

-- GDPR admin extras
GET    /admin/consents
GET    /admin/data-exports
GET    /admin/deletions
POST   /admin/gdpr/deletion-requests/:id/execute

-- Docs admin extras
GET    /admin/docs/:id
POST   /admin/docs
PATCH  /admin/docs/:id
DELETE /admin/docs/:id

-- Pages admin extras
GET    /admin/pages
GET    /admin/pages/:id
POST   /admin/pages
PATCH  /admin/pages/:id
DELETE /admin/pages/:id
POST   /admin/pages/:id/publish
POST   /admin/pages/:id/archive
PATCH  /admin/pages/:id/blocks/reorder
POST   /admin/pages/:id/save-as-template

-- Widgets admin
GET    /admin/widgets
POST   /admin/widgets
PATCH  /admin/widgets/:id
DELETE /admin/widgets/:id

-- SEO
GET    /admin/seo/validate/:content_id

-- API keys admin (will be moved to /control in P3)
GET    /admin/api-keys
POST   /admin/api-keys/:id/suspend

-- Analytics extras
GET    /admin/analytics/users
GET    /admin/analytics/content
GET    /admin/analytics/spaces
GET    /admin/analytics/revenue
GET    /admin/analytics/search
```

**Commit:** `docs(catalog): document admin routes present in code`

### Task 2.6: Missing user-facing routes → catalog

Add to the appropriate sections:

```
-- DM extras
GET    /dm/requests
POST   /dm/requests/:id/accept
POST   /dm/requests/:id/decline
POST   /dm/requests/:id/block        (NEEDS HANDLER — see Phase 3)

-- Content extras
POST   /content/:id/reactions
DELETE /content/:id/reactions/:key

-- Feed extras
GET    /feed/space/:space_id
GET    /feed/profile/:user_id

-- Media extras
GET    /media
GET    /media/:id
POST   /media/bulk-delete

-- Pages public
GET    /pages
POST   /pages
PATCH  /pages/:id
DELETE /pages/:id
PATCH  /pages/:id/blocks
GET    /page-templates
GET    /page-templates/:id
GET    /widgets/:id

-- Site settings
GET    /site/settings/public

-- Learning extras
GET    /learn/progress/:trackId

-- Subscriptions extras
GET    /subscriptions
POST   /subscriptions
DELETE /subscriptions/:id

-- Search
GET    /search/autocomplete

-- Content lifecycle (already in code)
POST   /content/:id/publish
POST   /content/:id/archive
POST   /content/:id/schedule
POST   /content/:id/view
GET    /content/:id/share-count
GET    /content/:id/related
GET    /content/:id/revisions
POST   /content/:id/pin
DELETE /content/:id/pin
GET    /reaction-types
PUT    /content/jsonld-templates
GET    /page-builder/block-types
POST   /content/render
GET    /link-preview
GET    /content/:id/bookmark
POST   /content/:id/bookmark
DELETE /content/:id/bookmark

-- Ads serving
GET    /ads/serve
POST   /ads/events

-- Onboarding
GET    /onboarding/status
PUT    /onboarding/step
POST   /onboarding/complete
GET    /onboarding/recommendations
POST   /onboarding/tour/dismiss

-- Push
GET    /push/vapid-key

-- WS
GET    /ws/online
PUT    /dm/privacy

-- AI extras
POST   /ai/generate
POST   /ai/hitl
GET    /ai/hitl/pending
POST   /ai/hitl/:id/approve
POST   /ai/hitl/:id/reject
GET    /admin/ai/tools
GET    /admin/ai/tools/:id
PUT    /admin/ai/tools/:id
GET    /admin/ai/tools/usage
GET    /admin/ai/context/:user_id
DELETE /admin/ai/context/:user_id/:entry_id
GET    /admin/ai/usage

-- Analytics extras
POST   /analytics/events
GET    /analytics/dashboard
GET    /admin/analytics/events
GET    /admin/analytics/overview
GET    /admin/analytics/security
POST   /admin/analytics/security/:id/resolve
POST   /admin/analytics/security/scan
POST   /admin/analytics/refresh
POST   /admin/analytics/cache/flush

-- Bot admin
GET    /admin/bots
POST   /admin/bots
PUT    /admin/bots/:id
DELETE /admin/bots/:id

-- Spam admin
GET    /admin/spam/ip-blocks
POST   /admin/spam/ip-blocks
DELETE /admin/spam/ip-blocks/:id
GET    /admin/spam/censored-words
POST   /admin/spam/censored-words
DELETE /admin/spam/censored-words/:id
GET    /admin/spam/user-blocks
DELETE /admin/spam/user-blocks/:blockerId/:blockedId

-- Newsletter admin
GET    /admin/newsletter/subscribers
GET    /admin/newsletter/subscribers/:id
DELETE /admin/newsletter/subscribers/:id
GET    /admin/newsletter/campaigns
GET    /admin/newsletter/campaigns/:id
POST   /admin/newsletter/campaigns
PATCH  /admin/newsletter/campaigns/:id
POST   /admin/newsletter/campaigns/:id/send

-- Referral admin
GET    /admin/referral/program
PUT    /admin/referral/program
GET    /admin/referral/referrals
GET    /admin/referral/referrals/:id

-- Mentorship admin
GET    /admin/mentorship/programs
GET    /admin/mentorship/programs/:id
POST   /admin/mentorship/programs
PATCH  /admin/mentorship/programs/:id
GET    /admin/mentorship/matches
PATCH  /admin/mentorship/matches/:id

-- Webhooks extras
POST   /admin/webhooks/:id/test
POST   /admin/webhooks/:id/retry/:delivery_id
PUT    /admin/webhooks/:id

-- Tickets extras
POST   /admin/tickets/:id/comments
POST   /admin/tickets/:id/resolve
GET    /admin/tickets/:id/events
POST   /admin/tickets/:id/notes
PUT    /admin/ticket-types/:id

-- Media admin extras
GET    /admin/media/quarantined
POST   /admin/media/:id/approve
POST   /admin/media/:id/reject
DELETE /admin/media/:id

-- Feature flags
PATCH  /admin/feature-flags/:flag_name

-- Settings
GET    /admin/settings
PUT    /admin/settings/:category

-- Developer
GET    /developer/swagger

-- Geo
GET    /geo/lookup
GET    /geo/lookup?ip=1.2.3.4
```

**Commit:** `docs(catalog): document remaining code-only user-facing and admin routes`

### Task 2.7: Bump .jeos submodule

**Commit (parent repo):** `chore: bump .jeos catalog (code-only route documentation)`

**Phase 1 gate:** Run `python3 scripts/route_diff.py` — CODE_ONLY should be 0 or near-zero (only intentional infra like `/health`, `/ready`, `/ws`).

---

## Phase 3: Remaining P2/P3 Renames

These are the actual code renames still pending. Each task = rename in server.go + update frontend callers + commit.

### Task 3.1: DM requests rename

**Code:** `POST /dm/message-requests/{id}/accept` → `POST /dm/requests/{id}/accept`
**Code:** `POST /dm/message-requests/{id}/decline` → `POST /dm/requests/{id}/decline`
**Add:** `POST /dm/requests/{id}/block` handler (does not exist yet — need to implement BlockMessageRequest in websocket/dm_handlers.go)

**Frontend:** `apps/main/src/lib/api.ts:455,460` — update paths

**Files:**
- `internal/server/server.go` (rename 2 routes + add 1)
- `internal/websocket/dm_handlers.go` (add BlockMessageRequest handler)
- `apps/main/src/lib/api.ts`

**Commit:** `refactor(server): rename dm/message-requests → dm/requests + add block handler`

### Task 3.2: Admin moderation rename

**Code:** `GET /admin/moderation` → `GET /admin/moderation-queue`
**Code:** `POST /admin/moderation/bulk` → `POST /admin/moderation-queue/bulk`
**Code:** `POST /admin/moderation/{id}/appeal` → `POST /admin/moderation-queue/{id}/appeal`

**Frontend:** `apps/admin/src/routes/moderation/+page.svelte:44,84,102` — update paths

**Files:**
- `internal/server/server.go`
- `apps/admin/src/routes/moderation/+page.svelte`

**Commit:** `refactor(server): rename admin/moderation → admin/moderation-queue`

### Task 3.3: Admin broadcast rename

**Code:** `POST /admin/broadcast` → `POST /admin/broadcasts`

**Frontend:** `apps/admin/src/routes/broadcast/+page.svelte:27` — update path

**Files:**
- `internal/server/server.go`
- `apps/admin/src/routes/broadcast/+page.svelte`

**Commit:** `refactor(server): rename admin/broadcast → admin/broadcasts`

### Task 3.4: DELETE /forum/:id/vote and /solve — add to catalog only

These are already at the correct paths in code. They just need documenting (done in Task 2.1 above). No code change needed — the catalog now has them.

### Task 3.5: Subscription cancel rename

**Code:** `PATCH /subscriptions/{id}/cancel` → `POST /me/subscription/cancel`
- The current code has `PATCH /subscriptions/{id}/cancel` using `paymentHandlers.CancelSubscription`
- Catalog target is `POST /me/subscription/cancel` — same handler, different method+path

**Frontend:** Need to grep for callers of `/subscriptions/` in apps.

**Files:**
- `internal/server/server.go` (remove the PATCH route, keep the existing POST /me/subscription/cancel which already exists)

**Commit:** `refactor(server): remove duplicate PATCH /subscriptions/:id/cancel (POST /me/subscription/cancel already exists)`

### Task 3.6: /me/preferences — resolve §6.3 dedupe

Investigation shows `/me/preferences` handles `dark_mode` + `language` (UI prefs) while `/me/notification-preferences` handles notification settings. These are **distinct**. Keep both and document in catalog (done in Task 2.4).

**Decision:** No rename needed — add to catalog only. The code is correct.

---

## Phase 4: Admin API Keys → Control Panel Move

### Task 4.1: Move admin API key routes from /admin to /control

**Code changes in `internal/server/server.go`:**
- `GET /admin/api-keys` → `GET /control/api-keys`
- `POST /admin/api-keys/{id}/suspend` → `POST /control/api-keys/{id}/suspend`
- Add: `POST /control/api-keys`, `GET /control/api-keys/:id`, `PATCH /control/api-keys/:id`, `DELETE /control/api-keys/:id`, `GET /control/api-keys/analytics`
- Update authz from `api.keys.view.all`/`api.keys.manage` to super_admin-level

**Frontend:** `apps/control/src/routes/api-keys/+page.svelte:26,36,46` — update paths

**Files:**
- `internal/server/server.go`
- `apps/control/src/routes/api-keys/+page.svelte`

**Commit:** `refactor(server): move admin api-keys → control/api-keys with super_admin authz`

---

## Phase 5: Remaining Missing User-Facing Routes (P4 subset)

These routes have existing service/handler methods but are not wired. Wire them.

### Task 5.1: /me/analytics/content + /me/analytics/followers

The users service has `GetUserAnalytics` (already wired at `GET /me/analytics`). The catalog wants separate `/me/analytics/content` and `/me/analytics/followers`. Check if the analytics service has granular methods or if this is a split of the existing aggregate.

**Decision needed:** Does `GetUserAnalytics` return both content+followers data? If so, either split the response or document the single endpoint as the implementation.

**Recommended:** Document `GET /me/analytics` as the implementation (it returns both). Update catalog to show `GET /me/analytics` instead of separate content/followers. Already wired.

**Commit:** `docs(catalog): align me/analytics to single aggregate endpoint`

### Task 5.2: /me/spaces/owned — defer

The spaces table has no `owner_id`/`created_by` column (geo-first model). The plan itself has a NOTE about this needing a schema decision. **Defer.**

### Task 5.3: /pages public routes

`GET /pages` (list) and `GET /pages/:slug` (already wired). Add `POST /pages`, `PATCH /pages/:id`, `DELETE /pages/:id` — these need new handler methods in the pagebuilder package that create/update/delete pages via the public API (distinct from admin CRUD).

**Files:**
- `internal/pagebuilder/handlers.go` (add CreatePagePublic, UpdatePagePublic, DeletePagePublic)
- `internal/server/server.go` (wire routes)
- `internal/pagebuilder/service.go` (may need new methods)

**Commit:** `feat(server): wire public page CRUD routes`

### Task 5.4: /page-templates public routes

`GET /page-templates` and `GET /page-templates/:id` — these are read-only public views. The pagebuilder service already has `ListTemplates` and `GetTemplate`. Wire them outside the admin group.

**Files:**
- `internal/server/server.go` (add public routes using existing handlers)

**Commit:** `feat(server): wire public page-templates routes`

### Task 5.5: /site/settings/public

Read `system_settings WHERE is_public`. The config.Settings service likely has a method for this. Wire it.

**Files:**
- `internal/server/server.go` (add route)
- Need to check if a handler exists or create one

**Commit:** `feat(server): wire /site/settings/public route`

### Task 5.6: /me/account PATCH

The users service likely has an account update method. Wire `PATCH /me/account` if the handler exists, or document that it's folded into `PATCH /me/profile`.

**Decision needed:** What fields does `/me/account` handle that `/me/profile` doesn't? (e.g., email, username, password changes). If the service has `ChangePassword` already wired at `POST /me/change-password`, then `/me/account` may be redundant.

**Recommended:** Defer until a product decision on what `/me/account` covers beyond existing endpoints.

### Task 5.7: /learn/quizzes/:id

Already wired: `GET /learn/quizzes/{id}` (public) and `POST /learn/quizzes/{id}/submit` (auth). Catalog matches. No action needed.

---

## Validation

After each phase, run:
1. `python3 scripts/route_diff.py` — verify CODE_ONLY shrinks
2. `go build ./...` — must be green
3. `go test ./internal/server/...` — must be green (new route guard test)
4. Frontend typecheck: `bun run check` for touched apps

Final gate (P6):
- `CODE_ONLY` should be 0 (or ≤5 intentional infra)
- `CATALOG_ONLY` should shrink by ~131 (all code routes documented)
- `go build ./... && go test ./...` green

---

## Open Questions / Deferrals

| Item | Status | Rationale |
|---|---|---|
| `GET /oauth/{provider}/initiate` → `POST /auth/oauth/:provider` | DEFERRED | Frontend uses `<a href>` GET links; handler issues 307 redirect. Needs frontend rework + full OAuth retest. |
| `GET /me/spaces/owned` | DEFERRED | No owner_id column in spaces table. Needs schema decision. |
| `PATCH /me/account` | DEFERRED | Unclear what fields this covers beyond existing /me/profile + /me/change-password. |
| `/admin/api-keys` → `/control/api-keys` authz elevation | DEFERRED to P3 | Catalog places in control panel (super_admin). Confirm authz change is acceptable. |
| `PUT /me/preferences` vs `PATCH /me/profile` | RESOLVED | Distinct: preferences = dark_mode+language, profile = display_name/avatar/bio/timezone. Keep both. |
