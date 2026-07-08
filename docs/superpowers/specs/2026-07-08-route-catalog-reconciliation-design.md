# API Route Catalog ↔ Code Reconciliation — Design

**Date:** 2026-07-08
**Author:** Backend reconciliation pass (post BACKEND-AUDIT.md)
**Status:** Approved direction, pending spec review

## 1. Goal

Bring the Go backend's registered Chi routes and the JEOS API routes catalog
(`.jeos/conventions/api-routes-catalog.md`) into exact agreement:

1. **Implement** every catalog route that has no handler in code.
2. **Document** every code route that is missing from the catalog.
3. **Rewrite** code route paths/param names to match the catalog where they
   diverge — the catalog path + param names are the standard.
4. **Zero duplicate routes** (Chi panics on duplicate `method+path`; several
   near-duplicates already exist and must be merged).

The catalog is authoritative. Where code is a *richer superset* of the catalog,
we keep the extra routes and add them to the catalog (never delete working
functionality).

## 2. Ground truth (authoritative diff)

A diff script (`scripts/route_diff.py`, seeded from the audit scratchpad)
normalizes Chi `{x}` and catalog `:x` to a common form and compares. Current
state:

| Metric | Count |
|---|---|
| Catalog route-entries (structural) | 574 |
| Code routes registered in `server.go` (excl. infra) | ~566 |
| Structurally matching | 190 |
| Catalog-only (implement or rename-target) | 384 |
| Code-only (document or rename-source) | 376 |

Numbers are produced by `scripts/route_diff.py`; re-run it to refresh.

**Note:** an earlier version of the harness under-captured routes registered in
the two-line `r.With(...).\n  Method(...)` form (321 of them), making code look
far sparser than it is. Corrected figures above show the code is nearly as rich
as the catalog — the divergence is dominated by **prefix/param renames** (a
renamed route appears once on each side) plus the genuinely-unimplemented
control/admin panels, not large swaths of missing user-facing functionality.

**Extraction caveats (accounted for):**
- Routes under `r.Route("/auth", …)` are stored relative (`/login`), so
  `login/register/refresh/logout/forgot-password/reset-password/verify-email/
  resend-verification/totp/*/oauth/*` are *matches*, not code-only.
- The `/api/v1` wrapper strips the base path, matching the catalog convention.
- `internal/seo` and `internal/auth/oauth_handlers.go` expose `Route(...)`
  helpers; only `/sitemap.xml` + direct oauth registration are actually wired.

`Chi {param}` ≡ `catalog :param`. A rename is only required when the param
**name/semantics** differ (e.g. `/forum/threads/{slug}` → `/forum/{id}`), not
for `{id}` vs `:id`.

## 3. Reconciliation principles

1. **Catalog path + param names are authoritative.**
2. **Superset domains → rename prefix, keep extras, document them** in the
   catalog. Applies to `comparison`, `professional-profiles`, `wallet`, `forum`,
   `blog`, `developer/keys`, `ai`.
3. **Zero duplicate routes.** When a code path is moved onto a catalog path that
   already partially exists, merge instead of re-registering.
4. **Auth/permission middleware is preserved** on every moved route; new routes
   copy the established `RequireAuth(kr, vk) [+ RequirePermission(...)]` pattern.
5. **Frontend follows.** Every renamed route has its `apps/*` (SvelteKit)
   callers updated in the same phase so nothing silently 404s.
6. **`[public]` means "reachable by a guest", authorization moves into the
   handler.** The catalog's `[public]` marker is not "no authz" — for several
   domains, access is **settings/resource driven** (§8). These routes use
   `OptionalAuth` (resolve identity if a token is present, never 401 a guest)
   and the handler applies the visibility/targeting rule. Only genuinely open
   routes (tags, geo, trending) use a bare public registration.

## 4. Bucket C — code-only routes → add to catalog (doc-only, zero risk)

Add these existing, working routes to `api-routes-catalog.md` under their
domain sections. Grouped (representative — full list is the code-only diff):

- **Auth/2FA:** `POST /auth/totp/setup|verify|disable` are being *moved* to
  `/me/totp/*` (§5); the code-only additions here are `GET /push/vapid-key`.
- **Custom fields & forms (user-facing):** `GET /custom-field-groups`,
  `/custom-field-groups/:id`, `/custom-field-groups/:id/fields`,
  `POST /custom-field-groups`, `PATCH|DELETE /custom-field-groups/:id`,
  `POST /custom-field-groups/:id/fields`, `PATCH|DELETE /custom-fields/:id`,
  `PUT /custom-fields/:id/permissions`,
  `GET /custom-field-values/:entity_type/:entity_id`, `PUT /custom-field-values`,
  `POST /forms/:id/submissions`, `GET /forms`, `GET /forms/:id`,
  `GET /forms/:id/submissions`.
- **Space types & space extras:** `GET /space-types`, `/space-types/:id`,
  CRUD; `GET/POST/PATCH/DELETE /spaces/:slug/rules`,
  `/spaces/:slug/parents`, `GET /space-permission-presets`,
  `PUT /space-permission-presets/:name`.
- **Content extras:** `GET /content` (list), `/content/:id/revisions`,
  `POST|DELETE /content/:id/pin`, `GET /content/:id/reactions`,
  `GET /reaction-types` + CRUD, `GET /content/jsonld-templates` +
  `PUT`, `GET /page-builder/block-types`, `POST /content/render`,
  `GET /link-preview`, `GET /content/:id/bookmark`.
- **Forum extras:** replies (`/forum/threads/:id/replies` → `/forum/:id/replies`
  after rename), `GET /forum/polls/:id`, forum tag CRUD, forum bookmarks.
- **Blog extras:** `GET /blog/reviews`, `POST /blog/reviews`,
  `PATCH /blog/reviews/:id`, `GET /blog/featured`, `POST /blog/featured`,
  `DELETE /blog/featured/:contentId`, `PATCH /blog/featured/reorder`,
  `GET /blog/feed.xml`.
- **Comparison / profiles extras** (superset — kept after prefix rename):
  comparison `types`, `dimensions`, `values`, `matrix`, `access`; profile
  `members`, `invitations`, `relocations`, `documents`, `verifiers`,
  `author-context`, `types`.
- **Wallet/payments extras:** `/me/wallet/ledger`, admin wallet `settings`,
  `rules/:id/toggle`, `admin/adjust`, `admin/user/:id/balance`;
  `GET /payments/:id`, `POST /payments/:id/refund`,
  `POST /subscriptions`, `GET /subscriptions`, discount-code admin CRUD.
- **Advertising extras:** campaign `activate/pause/end`, `boosts` CRUD.
- **Tickets extras:** `/me/tickets/:id/attachments` (GET/POST),
  admin ticket `notes`, `events`.
- **Ops/infra & misc:** `GET /health`, `GET /ready`, `GET /ws/online`,
  `GET /me/notification-types`, onboarding (`status/step/complete/
  recommendations/tour/dismiss`), `PUT /dm/privacy`,
  `POST /media/bulk-delete`, `GET /media`, `GET /media/:id`,
  `GET /admin/media/quarantined` + approve/reject, `POST /ai/generate`,
  AI `hitl/*`, `admin/ai/tools/*`, `admin/ai/context/*`,
  admin `bots/*`, `spam/*`, `referral/*`, `mentorship/*`, `newsletter/*`,
  `pagebuilder/blocks|themes`, `moderation`, `broadcast`,
  `developer/keys/:id/ip-allowlist/*`.

## 5. Bucket A — path/prefix renames (code has handler, catalog path differs)

Each row: **code (now) → catalog (target)**. Handler param-extraction is
updated where the param name changes. Frontend impact noted where a `apps/*`
app calls it.

### 5.1 User-facing (Phase 2)

| Code (now) | Catalog (target) | Notes / frontend |
|---|---|---|
| `GET /oauth/{provider}/initiate` | `POST /auth/oauth/:provider` | method+path; main login |
| `POST /auth/totp/setup` | `POST /me/totp/enable` | |
| `POST /auth/totp/verify` | `POST /me/totp/confirm` | |
| `POST /auth/totp/disable` | `DELETE /me/totp` | |
| `GET /users/me` | `GET /me` | main, admin |
| `PATCH /users/me` | `PATCH /me/profile` | main |
| `GET/PUT /users/me/preferences` | **dedupe** → `/me/notification-preferences` | see §6 |
| `POST /push/subscribe` | `POST /me/push-tokens` | main |
| `DELETE /push/subscribe` | `DELETE /me/push-tokens/:id` | main |
| `GET /users/blocked` | `GET /me/blocked-users` | main |
| `GET /users/muted` | `GET /me/muted-users` | main |
| `GET /users/spaces/muted` | `GET /me/muted-spaces` | main |
| `POST /account/export` + `POST /me/data-export` | **dedupe** → `GET /me/data-export` | see §6 |
| `POST /account/delete` + `DELETE /me` | **dedupe** → `DELETE /me` | see §6 |
| `GET /wallet/balance` | `GET /me/wallet` | main |
| `GET /wallet/ledger` | `GET /me/wallet/ledger` (+catalog) | |
| `POST /wallet/convert` | `POST /me/wallet/convert` | main |
| `GET /payments` | `GET /me/payments` | main |
| `GET /subscriptions/active` | `GET /me/subscription` | main |
| `PATCH /subscriptions/{id}/cancel` | `POST /me/subscription/cancel` | method+path; main |
| `POST /discount-codes/validate` | `POST /payments/discount-codes/validate` | |
| `POST /payments/initiate` | `POST /payments/intent` | main |
| `POST /forum/threads` | `POST /forum` | main |
| `GET /forum/threads` | `GET /forum` | main |
| `GET /forum/threads/{slug}` | `GET /forum/:id` | resolver: id-or-slug; main |
| `PATCH/DELETE /forum/threads/{id}` | `PATCH/DELETE /forum/:id` | main |
| `POST /forum/threads/{id}/solve` | `POST /forum/:id/solve` (+`DELETE`) | main |
| `POST /forum/vote` | `POST /forum/:id/vote` (+`DELETE`) | body→path; main |
| `POST /forum/polls` | `POST /forum/:id/poll` | main |
| `POST /forum/polls/{id}/vote` | `POST /poll-options/:option_id/vote` (+`DELETE`) | model check; main |
| `POST/DELETE /forum/threads/{id}/tags/{tagId}` | `POST /forum/:id/tags` / `DELETE /forum/:id/tags/:tag_id` | main |
| `POST /reviews` | `POST /reviews/:entity_type/:entity_id` | main |
| `GET /reviews` | `GET /reviews/:entity_type/:entity_id` | main |
| `GET /reviews/aggregate` | `GET /reviews/:entity_type/:entity_id/aggregate` | main |
| `/comparison/*` | `/comparisons/*` (prefix) | main |
| `GET /comparison/{id}` | `GET /comparisons/:slug` | id→slug check; main |
| `/professional-profiles/*` | `/profiles/*` (prefix) | main |
| `/learning/*` | `/learn/*` (prefix) | learn app |
| `GET /learning/certificates` | `GET /me/certificates` | learn |
| `GET /learning/certificates/{id}[/download]` | `GET /certificates/:id[/download]` | learn |
| `POST /ai/improve\|summarize\|translate\|generate-tags\|generate-og` | `POST /ai/assist/*` | main |
| `GET /i18n/locales` | `GET /i18n/packs` | all apps |
| `GET /translations/{locale}` | `GET /i18n/packs/:lang_code` | all apps |
| `GET /docs/{audience}` | `GET /docs/nav` | audience→query; docs |
| `GET /docs/api/openapi.json` | `GET /docs/openapi.json` | docs |
| `GET /developer/swagger[.json]` | **dedupe** → `/docs/openapi.json` | see §6 |
| `/developer/keys/*` | `/me/api-keys/*` (prefix) | control/docs |
| `POST /support` | `POST /tickets` | main |
| `GET /support/my[/{id}]` | `GET /me/tickets[/:id]` | main |
| `POST /support/my/{id}/reply` | `POST /me/tickets/:id/comments` | main |
| `GET /features` | `GET /feature-flags` | all apps |
| `GET /feed/primary` | `GET /feed` (+keep `/feed/primary`? no — rename) | main |
| `GET /feed/explore` | `GET /explore` | main |

### 5.2 Admin (Phase 3)

| Code (now) | Catalog (target) | Frontend |
|---|---|---|
| `/admin/advertising/campaigns*` | `/admin/ads/campaigns*` | admin |
| `/admin/advertising/slot-types*` | `/admin/ads/slot-types*` | admin |
| `GET /ads/render` | `GET /ads/serve` | main |
| `POST /ads/events/ingest` | `POST /ads/events` | main |
| `/admin/translations*` | `/admin/i18n/strings*` + `/admin/i18n/languages` | admin |
| `POST /admin/translations/batch` | `POST /admin/i18n/strings/ai-translate-batch` | admin |
| `/admin/pagebuilder/templates*` | `/admin/page-templates*` | admin |
| `/admin/pagebuilder/blocks\|themes*` | keep (+catalog) | admin |
| `/admin/learning/*` | `/admin/learn/*` | admin |
| `/admin/docs/{audience}` + CRUD | `/admin/docs/pages*` (catalog structure) | docs |
| `/admin/rbac/groups*` | `/admin/user-groups*` | admin |
| `/admin/rbac/roles*` | `/admin/roles*` | admin |
| `/admin/rbac/permissions` | keep under `/admin/roles`/catalog | admin |
| `/admin/moderation*` | `/admin/moderation-queue*` (catalog) + keep bulk/appeal | admin |
| `/admin/api-keys*` | `/control/api-keys*` (super-admin) — authz change, §8 | control |
| `/wallet/settings\|rules\|admin/*` | `/admin/wallet/*` | admin |

## 6. Deduplication cases (explicit)

Chi panics on duplicate `method+path`; these overlaps must be resolved to a
single registration:

1. **Data export:** `POST /account/export` (gdpr) **and** `POST /me/data-export`
   (users) both request an export. → Single `GET /me/data-export` (+ `/status`,
   `/download`). Pick the handler with the queue-backed export flow (gdpr
   service enqueues `data:export_user_data`); the users handler delegates or is
   removed.
2. **Account deletion:** `POST /account/delete` (gdpr) **and** `DELETE /me`
   (users) both request deletion. → Single `DELETE /me`. Keep the GDPR path's
   Honcho hard-erasure guarantee (§ audit Security). The users handler must call
   the gdpr service, not a second code path.
3. **Preferences:** `/users/me/preferences` (users) vs
   `/me/notification-preferences` (notifications). Confirm whether users'
   "preferences" == notification preferences; if so remove the users variant. If
   it's general profile prefs, fold into `PATCH /me/profile` / `PATCH /me/account`.
4. **OpenAPI spec:** `/developer/swagger[.json]` and `/docs/api/openapi.json`
   both serve the spec. → Single `GET /docs/openapi.json`.
5. **Feature flags list:** `GET /features` → `GET /feature-flags`; ensure no
   second `/feature-flags` is later added by admin toggle (that is
   `PATCH /admin/feature-flags/:flag_name`, distinct).

## 7. Bucket B — missing routes (full implementation)

### 7.1 Wiring existing services (Phase 4)

Service/query layer exists; only route + thin handler needed.

- **`/me` aggregations:** `/me/media`, `/me/spaces[/owned|/moderated]`,
  `/me/followed-tags`, `/me/following`, `/me/bookmark-folders` CRUD,
  `/me/reports`, `/me/analytics/content`, `/me/analytics/followers`,
  `/me/wallet/points`, `/me/wallet/credits`, `/me/api-keys/:id` (GET/PATCH),
  `/me/totp/status`, `/me/subscription/reactivate`, `PATCH /me/account`.
- **Spaces sub-views:** `GET /spaces/:id/content`, `/spaces/:id/feed` (alias to
  `/feed/space/:id`), `/spaces/:id/analytics`, `/spaces/:id/pinned` + POST +
  `DELETE /spaces/:id/pinned/:content_id` + `PATCH …/reorder`,
  `DELETE /spaces/:id/leave` (split from join toggle).
- **Users sub-views:** `GET /users/:id/content`, `/users/:id/analytics`,
  `/users/:id/follow-status`, `/users/:id/mute-status`,
  `POST/DELETE /users/:id/follow` (path form of `/follows`),
  `POST /users/:id/report`.
- **DM:** `GET /dm/requests`, `POST /dm/requests/:id/{accept,decline,block}`
  (rename from `/dm/message-requests/*`), `POST /dm/…/messages/:mid/report`.
- **Reporting:** `POST /content/:id/report`, `/spaces/:id/report`,
  `/comments/:id/report`.
- **Blog/forum solve/vote DELETE** counterparts; `GET /ai/conversations[/:id]`.
- **Sitemaps:** wire `sitemap-content|spaces|tags|users.xml` (SitemapHandler
  already has methods).
- **Pages public:** `GET /pages`, `POST /pages`, `PATCH/DELETE /pages/:id`,
  `PATCH /pages/:id/blocks`, `GET /page-templates[/:id]`, `GET /widgets/:id`.

### 7.2 Net-new subsystems (Phase 5)

No new core tables — all read-views over existing data unless noted.

- **Geo hierarchy** (`internal/geoip`): `/geo/countries`,
  `/geo/countries/:code/states`, `/geo/states/:id/cities`, `/geo/detect`.
  New sqlc read-queries over `spaces` filtered by geo space-types + parent
  hierarchy (catalog: "from seeded country spaces"). `/geo/detect` reuses the
  GeoIP reader.
- **Universities** (`internal/universities`, thin): `/universities`,
  `/universities/:slug`, `/universities/:id/compare`. Read-views over `spaces`
  where space-type = *University*, joined to `custom_field_values` from the
  *University Field Group* (per project owner: universities are spaces +
  custom-field group, **not** a new table). Compare reuses comparison service.
- **General tags** (`internal/tags`): `GET /tags`, `/tags/:slug`,
  `/tags/:slug/content`, `POST/DELETE /tags/:slug/follow`,
  `GET /tags/:slug/followers`. New read-queries over existing `tags` table;
  follow reuses `follows` service.
- **Content lifecycle** (`internal/content`): `POST /content/:id/publish`,
  `/archive`, `/schedule`, `/view`, `GET /content/:id/share-count`,
  `PATCH /content/:id/spaces`, `GET /content/:id/analytics`,
  `GET/POST /content/:id/discussions`. Extend content service against existing
  `content` table (status, scheduled_at, view counters).
- **Site settings:** `GET /site/settings/public` — reads
  `system_settings WHERE is_public`, existing settings service.
- **Feed aliases:** `GET /feed` (→primary), `GET /explore` (→explore),
  `GET /suggestions/content`.
- **Reactions rename:** `POST/DELETE /content/:id/react` (catalog) alongside the
  existing `GET /content/:id/reactions` list.

### 7.3 Admin & Control panel gaps (Phase 5, large workstream)

The catalog's admin section (§525–753) and the entire `/control/*` super-admin
panel (§760–819) are largely net-new in code. This is the biggest single chunk:

- **Admin gaps:** `/admin/users*` (list/detail/group/status/warn/ban/warnings/
  bans/ai-context), `/admin/content*` (list/remove/restore), `/admin/moderation-
  queue`, `/admin/reports*`, `/admin/appeals*`, `/admin/blog/review-queue*`,
  `/admin/spaces*` (status/join-requests/moderators), `/admin/tags*`,
  `/admin/reaction-types*` (admin), `/admin/notification-types*`,
  `/admin/broadcasts*`, `/admin/organizations/:id/verify`,
  `/admin/profiles/:id/verify`, `/admin/universities*`, `/admin/seo/validate/:id`,
  `/admin/gdpr/*`, `/admin/claim-ownership*`, `/admin/discount-codes/:id/usage`.
- **Control panel (all net-new):** tenants CRUD + suspend/unsuspend,
  system-settings, feature-flags, health/db/vacuum, queue stats/DLQ,
  cache stats/flush, security/ip-blocks, api-keys, ai-budget, audit-logs,
  deployments. New `internal/control` package with super-admin RBAC.

Where an admin capability already exists under a different code path (e.g.
`/admin/rbac/*` ↔ catalog `/admin/user-groups` + `/admin/roles`,
`/admin/spam/ip-blocks` ↔ `/control/security/ip-blocks`), reconcile by renaming
+ documenting rather than re-implementing.

## 8. Behavior changes flagged (not pure renames)

These change auth/method semantics and need explicit sign-off during their phase.
Confirmed with the project owner:

- **Spaces (`GET /spaces`, `GET /spaces/:slug`, and public sub-views):**
  visibility is **settings/resource driven — Private or Public per space**.
  Middleware → `OptionalAuth`; the handler reads the space's `visibility`
  (public → serve guests; private/restricted → require membership/auth and
  403/404 otherwise). Not a blanket public flag.
- **Users (`GET /users/:id`, `/users/:id/content`):** driven by the user's
  **privacy settings** (`user_privacy_settings`, migration 000106).
  `OptionalAuth` + per-profile privacy check (public → full; private → limited
  or 403). Not a blanket public flag.
- **Tags (`GET /tags`, `/tags/:slug`, `/tags/:slug/content`):** fully
  **public** — bare public registration.
- **Ads (`GET /ads/serve`, `POST /ads/events`):** reachable by guests but
  **eligibility/targeting is by usergroup via settings**. `OptionalAuth`; the
  serving logic filters eligible campaigns by the viewer's usergroup (guests →
  default/untargeted set). Drop the `ad.event.track` permission requirement on
  the public serve/ingest path.
- `POST /auth/oauth/:provider` → method change GET→POST for initiate.
- `/admin/api-keys` → `/control/api-keys` elevates the required role from admin
  to super_admin (catalog places API-key admin in the control panel). Proceed
  per catalog; flagged as an authz elevation.

## 9. Phase plan

Each phase is independently shippable and ends with build + route-diff +
targeted tests green.

- **P1 — Catalog additions (bucket C).** Doc-only edits to
  `api-routes-catalog.md`. No code risk.
- **P2 — User-facing renames (bucket A §5.1) + dedupe (§6) + frontend.**
- **P3 — Admin renames (bucket A §5.2) + admin/control-app frontend.**
- **P4 — Bucket B wiring (§7.1).**
- **P5 — Bucket B net-new (§7.2) + admin/control gaps (§7.3).** Largest;
  internally sub-sequenced: geo → universities → tags → content lifecycle →
  site settings → pages public → admin gaps → control panel.
- **P6 — Verification.** `go build ./...`, `route_diff.py` asserts code == catalog
  (empty diff except intentional infra routes), `go test ./...`, duplicate-route
  guard, frontend typecheck for touched apps.

## 10. Frontend coupling

Renamed routes are called from `apps/{main,learn,admin,control,docs}` (SvelteKit
`+page.server.ts` / `src/lib/api.ts`). Each rename phase greps the affected apps
for the old path and updates callers in the same commit. A per-phase grep list is
produced before editing so no caller is missed.

## 11. Verification harness

`scripts/route_diff.py` (promoted from scratchpad) is the single source of truth
for "code == catalog". It:
1. Parses `server.go` route registrations (handles the `/auth` sub-route + the
   `/api/v1` base).
2. Parses `api-routes-catalog.md` route lines.
3. Normalizes param names and prints catalog-only / code-only sets.

P6 passes when the only residual differences are intentional (root-path infra:
`/health`, `/ready`, `/ws`, sitemaps/OG at root). A `go test` in
`internal/server` walks the built Chi tree and asserts no duplicate `method+path`
and that every catalog route resolves.

## 12. Risks & open questions

- **Scope is very large** (≈150 net-new admin/control routes). P1–P4 remove the
  bulk of the *drift* the audit flagged; P5 is a multi-session workstream. Phases
  ship independently so value lands incrementally.
- **Superset identifier style:** spaces/content/forum are slug-addressed in code
  via resolvers, while the catalog mixes `:slug` (GET) and `:id` (mutations).
  Default: keep the slug resolver as the canonical identifier and treat catalog
  `:id` as "the space/content identifier"; document the deviation rather than
  add ID-based lookups that duplicate the resolver. Confirm during P2.
- **Behavior changes (§8)** are called out per phase for sign-off.
