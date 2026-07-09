# P5.6/P5.7 Completion — Admin Panel Gaps + Remaining Public Wiring

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development.
> This is the JIT sub-plan for the tail of Phase 5 in
> `docs/superpowers/plans/2026-07-08-route-catalog-reconciliation.md`.

**Context:** P1–P4 and most of P5 (net-new subsystems 5.1–5.5, control panel
5.7) are done — confirmed via `git log` and `python3 scripts/route_diff.py`
(`CODE_ONLY=0`, control panel fully wired with zero `/control/*` entries
remaining). What's left is **152 `CATALOG_ONLY` routes**: 99 under `/admin`,
53 public/frontend. A prior planning pass (`.kilo/plans/1783592340069-*.md`)
categorized these; spot-checking against the actual code found it partially
accurate — some claimed "existing handlers" don't exist under those names.
Treat every "wire existing" claim below as **unverified until the
implementer checks** — that check is part of the task, not a precondition.

**Goal:** `route_diff.py` → `CATALOG_ONLY=0` (modulo intentional infra),
matching the parent plan's Phase 5/6 gates.

## Global Constraints (inherited from parent plan)

- Catalog path + param **names** are authoritative; Chi `{x}` ≡ catalog `:x`.
- **Zero duplicate** `method+path` — `internal/server/routes_test.go` guards this.
- Preserve `RequireAuth`/`RequirePermission` middleware conventions already
  used for sibling routes in the same admin/public group.
- Settings-gated visibility (spaces/users/ads) uses `OptionalAuth` + handler
  check — do not blanket-public routes the design spec (§8) marks conditional.
- Every rename/new route updates its `apps/*` callers in the same commit
  **only if** a frontend caller already references the old/missing path;
  otherwise no frontend work is required (net-new admin capability with no
  UI yet is fine — catalog+backend only).
- After every task: `go build ./...` green, `python3 scripts/route_diff.py`
  showing the expected shrink, touched package tests green.
- Where a handler needs a genuinely new service/store method, follow the
  sibling method's pattern in the same file (naming, error handling,
  transaction use) rather than inventing a new convention.
- If a route implies a capability with no plausible thin implementation
  (e.g. `DBVacuum`-style infra actions) that already has a working analog
  elsewhere in the codebase, reuse it — don't build parallel infra.

## Task List

Each task = one implementer dispatch = one commit
(`feat(routes): P5.6 — <domain>` or `refactor(routes): P5.6 — <domain>`).
Run `python3 scripts/route_diff.py` before and after each task and record the
`CATALOG_ONLY` delta in the progress ledger.

### Task 1: customfields admin + form submit

19 routes. Admin field-groups CRUD (`/admin/field-groups`,
`/admin/field-groups/:id`, `/admin/field-groups/:id/fields`,
`/admin/field-groups/:id/fields/:id`,
`/admin/field-groups/:id/fields/reorder`), admin forms submissions
(`/admin/forms/:id/submissions`, `/admin/forms/:id/submissions/:id`
GET/PATCH), public `POST /forms/:id/submit`.
`internal/customfields` has `CreateFieldGroup/UpdateFieldGroup/
DeleteFieldGroup/CreateField` confirmed; `UpdateField/DeleteField/
ReorderFields/ListFormSubmissions` need checking — `ListFormSubmissions`
confirmed to exist.

### Task 2: comparison admin + public

11 routes. Admin comparison-types CRUD (confirmed: Create/Update/
DeleteComparisonType exist), public `GET /comparisons` (list),
`DELETE /comparisons/:id`, `PATCH /comparisons/:id`, `DELETE/PATCH
/comparisons/:id/items/:id` (confirmed: Update/DeleteComparisonItem exist —
just need wiring + list handler which may not exist).

### Task 3: docs admin + public

9 routes. Admin docs pages CRUD (confirmed: Create/Update/DeleteDocPage
exist) + approve/publish (check if a status-transition method exists or
needs adding), public `GET /docs/nav`, `GET /docs/search`, `POST
/admin/docs/ai-generate`.

### Task 4: notifications admin

7 routes. Admin notification-types CRUD (only `ListNotificationTypes`
confirmed — Create/Update/Delete need building), admin broadcasts
list/delete/send (net-new).

### Task 5: tickets admin

4 routes. `PATCH /admin/ticket-types/:id` (confirmed `UpdateTicketType`
exists), `DELETE /admin/tickets/:id` (check for a delete/soft-delete
method), `POST /admin/tickets/:id/comments` (confirmed `AddInternalNote`
exists — verify route shape matches), `POST /admin/tickets/:id/resolve`
(confirmed `ResolveTicket` exists).

### Task 6: rbac admin

3 routes. `DELETE /admin/roles/:id`, `PATCH /admin/roles/:id`, `PATCH
/admin/roles/:id/permissions`, `PATCH /admin/user-groups/:id`. None of
these confirmed present under these names in `internal/rbac` — check for
renamed equivalents before writing new ones.

### Task 7: tags admin

3 routes. `DELETE /admin/tags/:id/moderators/:id`, `POST
/admin/tags/:id/moderators`. `ListTags` confirmed; moderator add/remove
not confirmed — check `internal/tags` store for a moderators table/methods
before building new ones.

### Task 8: webhooks admin

3 routes. `PATCH /admin/webhooks/:id`, `POST
/admin/webhooks/:id/deliveries/:id/retry`, `POST
/admin/webhooks/:id/verify`. `internal/webhooks/types.go` exists but no
handlers confirmed — likely lives partly in `internal/payments/
webhooks.go`. Investigate before assuming net-new.

### Task 9: media admin

2 routes. `GET /admin/media`, `DELETE /admin/media/:id`.
`ListMedia`/`DeleteMedia` confirmed to exist — should be pure wiring with
an admin-permission middleware wrapper.

### Task 10: organizations admin

4 routes. `GET/PATCH /admin/claim-ownership[/:id]`, `POST
/admin/organizations/:id/verify`. `internal/organizations/organizations.go`
exists but no handlers file confirmed — check whether this is a thin
service needing a new handlers.go.

### Task 11: universities admin + public compare

5 routes. Admin universities CRUD + verify (net-new per design spec §7.2 —
universities are read-views over `spaces` + custom-field group, so admin
CRUD here means CRUD on the underlying space/field-values, not a new
table), public `GET /universities/:id/compare` (reuses comparison service
per spec).

### Task 12: spaces admin + public extras

6 routes. Admin `GET /admin/spaces/:id/join-requests`, `PATCH …/:id`,
`PATCH /admin/spaces/:id/status`; public `GET /spaces/:id/analytics`,
`PATCH /spaces/:id/pinned/reorder`, `GET /spaces/suggest-parents`.

### Task 13: professional profiles admin + public extras

5 routes. Admin `POST/DELETE /admin/profiles/:id/verify`; public `GET
/profiles/:id/content`, `POST /profiles/:id/members/invite`, `POST
/profiles/invitations/accept`.

### Task 14: learning admin

2 routes. `GET /admin/learn/tracks/:id/enrollments`, `POST
/admin/learn/tracks/:id/publish`.

### Task 15: payments/coupons admin + public subscription/subscriptions

4 routes. `PATCH /admin/discount-codes/:id`, `GET
/admin/discount-codes/:id/usage` (confirmed `GetDiscountCodeUsageStats`
exists — wire it), `DELETE /subscriptions/:id`, `POST
/me/subscription/reactivate`.

### Task 16: advertising admin

2 routes. `PATCH /admin/ads/campaigns/:id`, `GET
/admin/ads/slot-types/:id`.

### Task 17: wallet admin

2 routes. `PATCH /admin/wallet/rules/:id`, `POST /admin/wallet/award`.

### Task 18: blog admin review-queue

4 routes. `GET /admin/blog/review-queue`, `POST …/:id/approve`, `POST
…/:id/reject`. Net-new — check for existing post-status-transition methods
on the blog service before adding new ones (publish/unpublish patterns may
already exist to model from).

### Task 19: analytics admin

5 routes, all net-new. `GET
/admin/analytics/{content,revenue,search,spaces,users}`.
`internal/analytics` package exists — check what it already aggregates
(likely event-tracking data) before writing new query methods; keep each
endpoint to a single reasonably-scoped aggregate query, not a full BI
system.

### Task 20: gdpr admin

3 routes. `GET /admin/gdpr/deletion-requests`, `POST …/:id/execute`, `GET
/admin/gdpr/exports`. `internal/gdpr` exists — check for existing
request-listing methods (the user-facing `/me/data-export*` flow already
went through this package per P2.4).

### Task 21: users admin moderation

12 routes, net-new — largest single group. `GET/DELETE
/admin/users/:id/ai-context[/:id]`, `POST/DELETE /admin/users/:id/ban`,
`GET /admin/users/:id/bans`, `PATCH /admin/users/:id/group`, `PATCH
/admin/users/:id/status`, `POST /admin/users/:id/warn`, `GET
/admin/users/:id/warnings`, `DELETE /admin/users/:id/warnings/:id`. Model
ban/warning storage on whatever moderation-action pattern already exists
elsewhere (check `internal/admin` and `internal/reports` — moderation
queue likely already has an actions table this can extend rather than
duplicate).

### Task 22: appeals admin

3 routes. `GET /admin/appeals`, `GET /admin/appeals/:id`, `PATCH
/admin/appeals/:id`. No dedicated `appeals` package exists — check
`internal/reports` (moderation-adjacent) for the natural home before
creating a new package. Do this task after Task 21 — appeals may share a
moderation-action table with user warnings/bans; check what Task 21 built.

### Task 23: content admin moderation actions

2 routes. `POST /admin/content/:id/remove`, `POST
/admin/content/:id/restore`. Check `internal/content` for existing
soft-delete/status fields to model this on (likely reuses the same status
enum as content lifecycle P5.4).

### Task 24: affiliate admin

2 routes. `GET/PATCH /admin/affiliate-program/commission-rates`.
`internal/affiliate` exists — check current commission-rate storage (may
be a settings-table value).

### Task 25: i18n admin AI translate

2 routes. `POST /admin/i18n/strings/:id/translate`, `POST
/admin/i18n/strings/ai-translate-batch`. Check `internal/ai` for existing
AI-generation plumbing (P5's `ai/generate` work) to reuse rather than a
parallel integration.

### Task 26: moderation-queue assign + seo validate

2 routes. `PATCH /admin/moderation-queue/:id/assign`, `GET
/admin/seo/validate/:id`. `internal/seo` exists — check what it currently
validates.

### Task 27: pagebuilder admin extra + public pages CRUD + widgets

7 routes. `POST /admin/pages/:id/save-as-template`; public `GET/POST
/pages`, `PATCH/DELETE /pages/:id`, `PATCH /pages/:id/blocks`, `GET
/widgets/:id`.

### Task 28: me additions

6 routes. `PATCH /me/account`, `GET/PATCH /me/api-keys/:id`, `DELETE
/me/push-tokens/:id`, `GET /me/spaces/owned`, `POST
/me/subscription/reactivate` (dedupe with Task 15 if not already done
there — check before duplicating).

### Task 29: ai conversations

2 routes. `GET /ai/conversations`, `GET /ai/conversations/:id`. Check
`internal/ai` for existing conversation-history storage before assuming
net-new.

### Task 30: geo lookup + dm requests list

2 routes. `GET /geo/lookup?ip=` (P5.1 geo hierarchy already built the
GeoIP reader — reuse it), `GET /dm/requests` (list; the accept/decline/
block actions were already renamed in a prior commit per `ccab323` —
confirm this is just the missing list GET).

### Task 31: sitemaps

4 routes. `GET /sitemap-{content,spaces,tags,users}.xml`. Design spec
explicitly notes "SitemapHandler already has methods" — should be pure
wiring, cheapest-tier task.

### Task 32: forum extras

7 routes. `DELETE /forum/:id/solve`, `DELETE /forum/:id/vote`, `POST
/forum/:id/tags`, `POST /forum/:id/vote`, `POST /forum/:id/poll`,
`POST/DELETE /poll-options/:id/vote`. `SolveThread`, `Vote`, `CreatePoll`,
`VotePoll` all confirmed to exist in `internal/forum` — this should be
mostly wiring plus DELETE counterparts (unvote/unsolve) which may need new
thin methods mirroring the POST ones.

### Task 33: content public extras

6 routes. `GET /content/:id/analytics`, `GET/POST /content/:id/discussions`,
`POST/DELETE /content/:id/react`, `PATCH /content/:id/spaces`. None of
React/Unreact/ContentAnalytics/discussions confirmed to exist — likely
net-new, but check for a generic reactions-service shared with
forum/blog before building a parallel one (the catalog already has `GET
/content/:id/reactions` working, so a reactable-entity abstraction may
already exist).

### Task 34: users public extras

4 routes. `GET /users/:id/analytics`, `GET /users/:id/content`,
`POST/DELETE /users/:id/follow` (path-form of the existing `/follows`
body-form — reuse the same follow service method, just a second route
shape per design spec §7.1).

### Task 35: suggestions/content (trending)

1 route. `GET /suggestions/content`. Check for an existing
trending/suggestion service.

**Final gate for this sub-plan:** `python3 scripts/route_diff.py` →
`CATALOG_ONLY=0` (or only entries the team explicitly defers, documented in
the parent plan's Phase 6 completion notes). Then proceed to the parent
plan's Phase 6 verification tasks (6.1–6.5).

## Sequencing note

Tasks are ordered roughly by confidence (verified-existing-handler tasks
first, net-new-heavy tasks later) and by dependency risk (T21 users
moderation and T22 appeals may share a moderation-action table — do T21
before T22 so T22's implementer can check what T21 built). Otherwise tasks
are independent; execute sequentially since all touch `server.go`.
