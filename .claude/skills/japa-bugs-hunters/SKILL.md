---
name: japa-bugs-hunters
description: JapaConnect comprehensive bug hunter. On load it presents a menu — (1) a frontend URL or page name of any of the 5 apps, (2) a backend API route, or (3-8) a whole surface (backend / main / control / admin / docs / learn). It hunts bugs from the JapaConnect Bug Hunter's Guide (Prompts/japa-bugs-hunters.md) against the page/route/surface, fixes frontend issues, and — when a bug needs backend work (missing route, unwired handler, incomplete CRUD, migration, RBAC, Valkey, Asynq, GDPR) — fixes the backend too until the target is 100% working. Use when the user says "hunt bugs", "bug hunt", "audit and fix", "fix bugs on <page/route>", or triggers /japa-bugs-hunters.
---

# JapaConnect Bug Hunter

This skill turns the **JapaConnect Bug Hunter's Guide** (`Prompts/japa-bugs-hunters.md`, co-located in
the repo root) into an actionable, self-fixing workflow. The Guide is the taxonomy of bugs; this
skill is the engine that applies it.

**Authoritative business truth:** the `.jeos/` repo — `JEOS.md`, `sections/*`, `conventions/*`
(api-routes-catalog, permission-key-naming, system-settings-catalog, valkey-keys-catalog,
asynq-tasks, api-response-format, database-migration-naming, feature-flag-naming,
svelte-component-patterns), the 60 ADRs, and `AGENTS.md` **17 Golden Rules**. Any code that
contradicts JEOS is a bug even if it compiles.

## Critical Environment Notes (Always Add these to context)

**These must be followed every time this skill is loaded:**

1. **Always use Playwright** whenever a page URL or link is supplied. This is mandatory for all
   frontend testing, screenshot capture, and browser interaction. Never skip browser automation.

2. **Chrome is already installed** on this system. Do NOT attempt to install Chromium — it is
   unnecessary and will waste time. Use the existing Chrome installation.

3. **Playwright is already installed** on this system. Do NOT run `npx playwright install` or any
   similar installation command. The system is pre-configured and ready to use.

4. **Super admin credentials:** `admin@japaconnect.com` / `Admin123!` — use these when login is
   required to test authenticated pages or admin features.

5. **Always use `bun`** to start dev servers, install packages, or run any app commands. Never use
   `npm` or `pnpm` — this project uses Bun as its runtime and package manager. When a server or
   app is down and needs to be started for testing, use `bun dev` (or `bun run dev`) in the
   relevant app directory, never `npm run dev` or `pnpm dev`.

6. **Always ask the user before closing the browser.** Before running `agent-browser close` or
   ending the browser session, ask the user to verify whether the issue is fully resolved or if
   there are remaining bugs to fix. Never close the browser unilaterally — the user may want to
   test additional scenarios or see the fix in action first.

## Step 0 — On Load: Present the Menu

Immediately after this skill loads, use the **`question`** tool to present these options and let the
user pick exactly one. Use `multiple: false`.

```
header: "Target"
question: "What do you want to bug-hunt and fix?"
options:
  - label: "Frontend URL / page"      (option 1)
      A specific page on any of the 5 apps. You'll supply a URL (or app+page name); the agent loads
      Playwright, tests the page against relevant bugs, fixes it, and fixes any backend the page needs.
  - label: "Backend API / route"      (option 2)
      A specific /api/v1/* route or handler. The agent reviews/debugs it for bugs, unimplemented
      features, incomplete wiring, missing migrations, RBAC, etc., and fixes them.
  - label: "Backend (whole)"          (option 3)
      Audit + fix the entire Go API monolith against the Guide.
  - label: "Main app"                 (option 4)
      Audit + fix apps/main (port 3000, SSR) against the Guide.
  - label: "Control panel"            (option 5)
      Audit + fix apps/control (port 3002, SPA) against the Guide.
  - label: "Admin panel"              (option 6)
      Audit + fix apps/admin (port 3001, SPA) against the Guide.
  - label: "Docs site"                (option 7)
      Audit + fix apps/docs (port 3004, SSR) against the Guide.
  - label: "Learn platform"           (option 8)
      Audit + fix apps/learn (port 3003, SSR) against the Guide.
```

After the user answers, read `Prompts/japa-bugs-hunters.md` in full, then execute the matching
workflow below. **Never skip reading the Guide** — it contains the section-number references you must
cite in fixes and reports.

---

## Option 1 — Frontend URL / Page

1. **Resolve the app from the URL** (or ask which app if only a page name is given):
   - `localhost:3000` / `japaconnect.com` → **main** (SSR, ADR-058 shell)
   - `localhost:3001` / `admin.*` → **admin** (SPA)
   - `localhost:3002` / `control.*` → **control** (SPA)
   - `localhost:3003` / `learn.*` → **learn** (SSR, ADR-060 lesson-player exception)
   - `localhost:3004` / `docs.*` → **docs** (SSR, ADR-060 nav-tree/TOC)
2. **Launch Playwright** (use the `webapp-testing` skill / `playwright-cli`). Start the dev server
   for that app if not running. Open the URL, capture console errors, network failures (esp. 4xx/5xx
   on `/api/v1/*`), layout/visual screenshots at desktop + mobile widths, and hydration warnings.
3. **Filter the Guide to the page.** Apply the `[FE]` and `[BOTH]` sections relevant to the page
   type: §1 (forms/nav/calc), §2 (UI/UX, dark mode, LayerChart), §3 (API calls, 401 loop, contract
   drift), §6 (Svelte 4 anti-patterns), §7 (i18n), §16 (SSR/hydration), §17 (admin/control SPA),
   §21 (stub/missing UI), §23 (form data), §27 (Rule 16 hardcoded data).
4. **Fix frontend bugs** in place (Svelte 5 runes, Bits UI, LayerChart 2.0.1, `var(--chart-*)` theming,
   `$t()` i18n, no raw hex, no `export let`/`$:`/`on:`).
5. **If a bug needs the backend, fix the backend too — do not stop at the frontend.** This includes:
   - A route the page calls that **does not exist** → add it (see Option 2 workflow): pick the owning
     `sections/*` file, add the route to `conventions/api-routes-catalog.md`, implement
     handler→service→store with sqlc, add any migration, wire RBAC node + CSRF, return the standard
     error envelope.
   - An **unwired/incomplete handler** (stub, no-op, missing CRUD leg) → implement it fully.
   - Missing **RBAC node / permission cache invalidation**, **Valkey key** (must be `jc:`-prefixed +
     tenant-scoped + cataloged), **Asynq task** (must be in asynq-tasks catalog), **system_settings**
     key, or **migration** → add/fix per JEOS.
   - **GDPR** gap if the page surfaces PII → cover the table in erasure.
6. Re-run Playwright on the page; iterate until **100% working** (no console errors, all network
   calls succeed, layout correct light+dark, i18n present, no hydration mismatch).

## Option 2 — Backend API / Route

1. Locate the handler in `internal/` (match route → package → `server.go`/`handlers`). Confirm it is
   registered in chi and matches `conventions/api-routes-catalog.md`.
2. Review the **full vertical slice**: route → handler → service → store (sqlc) → migration.
3. Apply the `[BE]` and `[BOTH]` sections: §1.3 (calc), §3 (API contract, error envelope, idempotency),
   §4 (queries, Valkey tenant-scoping, N+1, pgvector), §5 (SQLi, CSRF, RBAC, secrets), §6.2/6.3 (Go
   logic, nil deref), §8 (migrations, circular FK, sqlc regen), §9 (RBAC/multi-tenancy), §10 (Asynq
   catalog + idempotency + DLQ), §11 (WebSocket), §12 (Meilisearch tenant filter), §13 (payments/
   wallet ledger + `SET NX EX`), §14 (`content_hash`, soft-delete, Tiptap sanitize), §15 (GDPR erasure
   coverage), §19 (AI `ai.context.*` default-off, budget), §21 (stub/incomplete handler, missing CRUD
   leg), §22 (the 17 Golden Rules), §24 (field/data types), §25 (custom fields), §26 (RBAC CRUD +
   wiring).
4. **Fix:** implement missing logic, complete unwired handlers, add the missing migration/sqlc query
   (edit `db/queries/`, run `make sqlc-gen` — never hand-edit `internal/db/`), add RBAC node + cache
   invalidation, add Valkey key to catalog, register Asynq task in catalog, enforce `system_settings`,
   return standard `{ "data": ... }` / `{ "error": { "code", "message" } }`.
5. Verify the route returns non-404/non-500 under correct auth and matches the section's contract.

## Options 3–8 — Whole Surface Audit + Fix

Map the surface to its bug sections, then audit file-by-file and fix:

- **Option 3 — Backend (whole):** all `[BE]`/`[BOTH]` sections above; prioritize §21 (stub/missing
  routes — every one of the ~430 catalogued routes must be reachable & non-stub), §22 (17 Golden
  Rules), §26 (RBAC node reconciliation), §9 (tenant scoping), §10 (Asynq catalog). Use the
  `japa-backend-auditor` skill's route/node/catalog reconciliation approach as a sub-routine.
- **Option 4 — Main (apps/main):** §1, §2, §3, §6, §7, §16 (SSR/hydration), §17 (where shared), §21,
  §27 (Rule 16). Enforce ADR-058 shell, LayerChart 2.0.1, Bits UI, `$t()` i18n, no hardcoded data,
  JSON-LD/SEO. Use `jc-main-audit` patterns.
- **Option 5 — Control (apps/control):** §2, §3, §6, §7, §16 (SPA), §17 (SPA specifics: auth guard
  loop, polling leak, Bits UI key handling, LayerChart `xPadding` not `xNice`), §21, §27. Use
  `jc-control-audit` patterns.
- **Option 6 — Admin (apps/admin):** §2, §3, §6, §7, §16 (SPA), §17, §21, §23 (forms), §25 (custom
  fields), §26 (RBAC wiring vs sidebar nav map), §27. Use `jc-admin-audit` patterns.
- **Option 7 — Docs (apps/docs):** §1, §2, §3, §6, §7, §16 (SSR, ADR-060 nav-tree/TOC), §18, §21, §27.
- **Option 8 — Learn (apps/learn):** §1, §2, §3, §6, §7, §16 (SSR, ADR-060 lesson-player exception),
  §18, §21, §27, §36 (learning routes).

For each surface, produce the structured report (see below) and apply fixes until clean. Re-run the
dev server / `svelte-check` / `go build ./...` / `gosec` as appropriate to verify.

---

## Cross-Cutting Fix Rules (must hold for every fix)

- **Golden Rules (`AGENTS.md`):** section is source of truth (R1); never hand-edit `internal/db/` (R2);
  config from `JC_*`/`system_settings`, never hardcoded (R3); Valkey keys `jc:`-prefixed + cataloged
  (R4); standard error envelope (R5); invalidate `jc:perm:{user_id}` on group/role/mod/ban changes (R6);
  Asynq task name in catalog (R7); fan-out fire-and-forget (R8); `SET NX EX` idempotency on financial
  endpoints (R9); impersonation writes `audit_logs` (R10); soft-delete only in V1 (R11); compute
  `content_hash` before every `contents` save (R12); circular-FK migration ordering (R13); read all
  referenced sections (R14); all ~430 routes return non-404 under auth (R15); frontend API-only, no
  hardcoded/mock data (R16); no AI authorship attribution (R17).
- **RBAC:** every `hasPermission()` node must exist in the `permissions` seed using `_own`/`_any`
  scoping; `super_admin` holds all nodes; `members` holds exactly the 45 member-level nodes.
- **Multi-tenancy:** every query, Valkey key, and Meilisearch filter carries `tenant_id`.
- **Custom fields:** correct `field_type`, `options` jsonb shape, RBAC-per-field scoping, valid
  `entity_class` assignment.

## Report Format

End every run with:

```
## Bug Hunt Report — <target>
- Mode: <option 1-8> · App/Surface: <...> · Route(s): <...>
- Bugs found: <n>  (Frontend: <n> · Backend: <n>)
- Fixed: <n>  · Remaining: <n> (with reason if any)
- Backend changes: routes added <list>, migrations <list>, RBAC nodes <list>, Valkey keys <list>,
  Asynq tasks <list>
- Files changed: <list>
- Verification: <Playwright pass / go build / svelte-check / gosec result>
- Checklist A (JEOS Conformance) and B (Feature-completeness) from the Guide: ✅/❌ per item
```

## Reference Files

- `Prompts/japa-bugs-hunters.md` — the full bug taxonomy (read first, every run).
- `.jeos/JEOS.md` — index of all sections, ADRs, conventions.
- `.jeos/AGENTS.md` — the 17 Golden Rules.
- `.jeos/conventions/api-routes-catalog.md` — ~430 routes (existence/contract check).
- `.jeos/conventions/permission-key-naming.md`, `system-settings-catalog.md`,
  `valkey-keys-catalog.md`, `asynq-tasks.md`, `api-response-format.md`.
- `.jeos/sections/03-rbac.md`, `sections/06-custom-field-builder-form-builder.md`.
