# API Route Catalog ↔ Code Reconciliation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `internal/server/server.go` Chi routes and
`.jeos/conventions/api-routes-catalog.md` agree exactly — implement missing
routes, document code-only routes, rename divergent paths to the catalog
standard, with zero duplicates.

**Architecture:** Six phases (P1 doc additions → P2 user renames → P3 admin
renames → P4 wiring → P5 net-new → P6 verify). `scripts/route_diff.py` is the
completeness gate: reconciliation is done when its `CATALOG_ONLY` /
`CODE_ONLY` sets contain only intentional infra routes. Spec:
`docs/superpowers/specs/2026-07-08-route-catalog-reconciliation-design.md`.

**Tech Stack:** Go 1.26, Chi v5, sqlc + pgx/v5, PostgreSQL 18, SvelteKit
frontends in `apps/*`. Catalog lives in the `.jeos` git submodule.

## Global Constraints

- Catalog path + param **names** are authoritative; Chi `{x}` ≡ catalog `:x`.
- Superset domains: rename prefix, **keep** extra routes, **add** them to catalog.
- **Zero duplicate** `method+path` (Chi panics; `internal/server` test guards this).
- Preserve `RequireAuth`/`RequirePermission` middleware on every moved route.
- Settings-gated visibility (spaces/users/ads) uses `OptionalAuth` + handler check.
- Every rename updates its `apps/*` callers in the same commit.
- After every phase: `go build ./...` green, `python3 scripts/route_diff.py`
  showing the expected shrink, touched package tests green.
- Catalog edits are committed **inside the `.jeos` submodule**, then the parent
  repo's submodule pointer is bumped.

---

## File structure

| File | Responsibility | Phases |
|---|---|---|
| `internal/server/server.go` | route registration | P2–P5 |
| `.jeos/conventions/api-routes-catalog.md` | canonical catalog | P1–P5 |
| `internal/server/routes_test.go` (new) | duplicate-route + catalog-resolves guard | P6 (seed P1) |
| `scripts/route_diff.py` | code↔catalog diff gate | all |
| `internal/{geoip,universities,tags,content,...}` | net-new handlers/services | P5 |
| `internal/db/queries/*.sql` | new sqlc read-queries | P5 |
| `apps/{main,learn,admin,control,docs}/**` | frontend callers | P2–P5 |

---

## Phase 1 — Catalog additions (bucket C, doc-only)

**Deliverable:** every code-only route documented in the catalog;
`route_diff.py` `CODE_ONLY` drops to only intentional infra.

### Task 1.1: Seed the route guard test

**Files:** Create `internal/server/routes_test.go`

- [ ] **Step 1:** Write a test that builds the server router and asserts the
  Chi tree has no duplicate `method+path` and walks cleanly.

```go
package server

import (
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestNoDuplicateRoutes walks the built router and fails on any duplicate
// method+path — Chi would panic at registration, but this documents the intent
// and lets the diff harness assume a clean tree.
func TestNoDuplicateRoutes(t *testing.T) {
	seen := map[string]bool{}
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		if seen[key] {
			t.Errorf("duplicate route: %s", key)
		}
		seen[key] = true
		return nil
	}
	r := chi.NewRouter()
	// NOTE: full NewServer needs a DB; this guard runs in P6 with a test DB.
	// For P1 it is a compile-time placeholder asserting the walk signature.
	_ = walk
	_ = r
}
```

- [ ] **Step 2:** `go build ./internal/server/` — expect PASS (compiles).
- [ ] **Step 3:** Commit: `test(server): seed route guard scaffold`.

### Task 1.2: Add code-only routes to catalog, domain by domain

**Files:** Modify `.jeos/conventions/api-routes-catalog.md`

Work through the `route_diff.py` `CODE_ONLY` list. For each domain block, add
the missing lines under the matching catalog section (see spec §4 for the
grouped list). Do it in these sub-commits so each is reviewable:

- [ ] **1.2a** Custom fields & forms, space-types, space rules/parents/presets.
- [ ] **1.2b** Content extras (revisions, pin, reactions list, jsonld-templates,
  block-types, render, link-preview, bookmark-status).
- [ ] **1.2c** Forum extras (replies, poll GET, tag CRUD, bookmarks) — mark these
  with their **post-rename** paths (`/forum/:id/replies`) so P2 doesn't re-touch.
- [ ] **1.2d** Blog extras (reviews, featured), reviews `:id`/flag.
- [ ] **1.2e** Comparison/profile superset extras (post-rename prefixes).
- [ ] **1.2f** Wallet/payments/subscriptions/discount admin extras.
- [ ] **1.2g** Advertising campaign actions + boosts; tickets attachments/notes/events.
- [ ] **1.2h** Onboarding, push/vapid-key, media list/detail/bulk/quarantine,
  dm/privacy, ws/online, notification-types.
- [ ] **1.2i** AI generate/hitl/tools/context; admin bots/spam/referral/mentorship/
  newsletter/pagebuilder-blocks-themes/moderation/broadcast; developer ip-allowlist.

After each sub-commit run `python3 scripts/route_diff.py` and confirm
`CODE_ONLY` shrinks. Commit inside `.jeos`:
`docs(catalog): document <domain> routes present in code`.

- [ ] **Step final:** In parent repo, `git add .jeos` and commit
  `chore: bump .jeos catalog (code-only route documentation)`.

**Phase 1 gate:** `route_diff.py` `CODE_ONLY` contains only intentional infra
(root sitemaps/OG/RSS handled at root, `/health`, `/ready`).

---

## Phase 2 — User-facing renames (bucket A §5.1) + dedupe (§6)

**Deliverable:** user-facing code paths match the catalog; duplicates merged;
frontend callers updated. Each domain is one task: rename in `server.go`
(+ handler `chi.URLParam` if the param name changed) → grep+update `apps/*` →
build → diff.

Task list (each = its own commit, TDD where a handler body changes):

- [ ] **2.1** Auth/TOTP → `/me/totp/*`; `oauth/:provider/initiate` → `POST /auth/oauth/:provider`.
- [ ] **2.2** `/users/me*` → `/me`, `/me/profile`; dedupe `/users/me/preferences` (§6.3).
- [ ] **2.3** Push → `/me/push-tokens*`; blocked/muted lists → `/me/blocked-users`, `/me/muted-*`.
- [ ] **2.4** Data-export + deletion dedupe (§6.1, §6.2) → `GET /me/data-export*`, `DELETE /me` on the GDPR service path.
- [ ] **2.5** Wallet → `/me/wallet*`.
- [ ] **2.6** Payments → `POST /payments/intent`, `GET /me/payments`, subscriptions → `/me/subscription*`, discount validate → `/payments/discount-codes/validate`.
- [ ] **2.7** Forum → `/forum` + `/forum/:id/*`, poll-options vote, tags (verify handler id/slug + vote body→path).
- [ ] **2.8** Reviews → `/reviews/:entity_type/:entity_id[/aggregate]`.
- [ ] **2.9** Comparison → `/comparisons` prefix (verify id vs slug on GET).
- [ ] **2.10** Professional-profiles → `/profiles` prefix.
- [ ] **2.11** Learning → `/learn` prefix; certificates → `/certificates/:id`, `/me/certificates`. (learn app)
- [ ] **2.12** AI assist → `/ai/assist/*`.
- [ ] **2.13** i18n → `/i18n/packs*`; docs → `/docs/nav`, `/docs/openapi.json`; dedupe developer/swagger (§6.4).
- [ ] **2.14** Developer keys → `/me/api-keys*`.
- [ ] **2.15** Tickets → `/tickets`, `/me/tickets*`.
- [ ] **2.16** Feature flags `/features` → `/feature-flags`; feed `/feed/primary`→`/feed`, `/feed/explore`→`/explore`.

Per task: `python3 scripts/route_diff.py` must show `CATALOG_ONLY` and
`CODE_ONLY` both shrink (route moved from one side to matching).

**Phase 2 gate:** `go build ./...`, `go test ./internal/...` for touched
packages, no user-facing rename left in `CODE_ONLY`.

---

## Phase 3 — Admin renames (bucket A §5.2)

One task per domain, same pattern (rename → admin/control frontend → verify):
`/admin/advertising`→`/admin/ads`; `ads/render`→`/ads/serve`,
`ads/events/ingest`→`/ads/events`; `/admin/translations`→`/admin/i18n/*`;
`/admin/pagebuilder/templates`→`/admin/page-templates`;
`/admin/learning`→`/admin/learn`; `/admin/rbac/*`→`/admin/user-groups`+`/admin/roles`;
`/admin/moderation`→`/admin/moderation-queue` (+keep bulk/appeal);
`/admin/api-keys`→`/control/api-keys` (authz elevation, §8).

**Phase 3 gate:** admin diff entries resolved; admin/control apps build.

---

## Phase 4 — Bucket B wiring (§7.1)

Routes whose service/query already exists — TDD each (handler test → wire →
green). Groups: `/me` aggregations; spaces sub-views + join/leave split;
users sub-views + path-form follow; dm/requests rename + report; reporting
endpoints; sitemap variants; public pages CRUD; `GET /ai/conversations*`.

**Phase 4 gate:** these disappear from `CATALOG_ONLY`; new handler tests green.

---

## Phase 5 — Net-new subsystems (§7.2, §7.3)

Each subsystem gets a **just-in-time detailed sub-plan** (own file in
`docs/superpowers/plans/`) written immediately before implementing it, because
each needs migration/sqlc/service/handler/test decomposition. Order:

- [ ] **5.1** Geo hierarchy (read-views over `spaces`).
- [ ] **5.2** Universities (spaces + University field-group read-views).
- [ ] **5.3** General tags subsystem.
- [ ] **5.4** Content lifecycle (publish/archive/schedule/view/analytics/discussions).
- [ ] **5.5** Site settings public; feed/explore/suggestions aliases; content react rename.
- [ ] **5.6** Admin panel gaps (users/content/moderation/reports/appeals/…).
- [ ] **5.7** Control panel (`internal/control`, super-admin) — tenants, settings,
  flags, health, queue, cache, security, api-keys, ai-budget, audit-logs, deploys.

**Phase 5 gate:** `CATALOG_ONLY` empty except intentional infra.

---

## Phase 6 — Verification

- [ ] **6.1** Flesh out `internal/server/routes_test.go` with a test DB, assert no
  duplicate routes and that every catalog route resolves in the Chi tree.
- [ ] **6.2** `python3 scripts/route_diff.py` → `CATALOG_ONLY=0 CODE_ONLY=0`
  (modulo the documented infra allowlist).
- [ ] **6.3** `go build ./... && go test ./...` green.
- [ ] **6.4** Frontend: `bun run check` (or per-app typecheck) for touched apps.
- [ ] **6.5** Update BACKEND-AUDIT.md route section to "resolved"; commit.

---

## Self-review

- **Spec coverage:** P1↔§4, P2↔§5.1+§6, P3↔§5.2, P4↔§7.1, P5↔§7.2+§7.3,
  P6↔§11; behavior changes §8 handled in their phase. Covered.
- **Placeholders:** P5 sub-tasks intentionally defer to JIT sub-plans (large
  independent subsystems, per writing-plans scope-check) — not placeholder
  hand-waving but explicit decomposition points with defined scope.
- **Harness consistency:** every phase gate is expressed as a `route_diff.py`
  delta, so "done" is objective, not judgment.
