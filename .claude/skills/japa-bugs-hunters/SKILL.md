---
name: japa-bugs-hunters
description: JapaConnect comprehensive bug hunter. On load it presents a menu — (1) a frontend URL or page name of any of the 5 apps, (2) a backend API route, (3-8) a whole surface (backend / main / control / admin / docs / learn), (9) a feature name from ./internal (e.g. forum, like, bookmark, ai, affiliate) for a full cross-stack audit against JEOS Specs with findings saved to ./.jeos/audit, or (10) apply fixes to a feature's audit findings in ./.jeos/audit/<feature_name>-audit.md. It hunts bugs from the JapaConnect Bug Hunter's Guide (Prompts/japa-bugs-hunters.md) against the page/route/surface/feature, fixes frontend issues, and — when a bug needs backend work (missing route, unwired handler, incomplete CRUD, migration, RBAC, Valkey, Asynq, GDPR) — fixes the backend too until the target is 100% working. Use when the user says "hunt bugs", "bug hunt", "audit and fix", "fix bugs on <page/route>", "audit feature <name>", "audit [feature_name]", "fix [feature_name]", "audit /api/v1/<route>", "fix /api/v1/<route>", "audit <url-or-path>", "fix <url-or-path>", "audit <surface>", "fix <surface>" (surface = backend/main/control/admin/docs/learn), or triggers /japa-bugs-hunters.
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
  - label: "Feature name (from ./internal)"  (option 9)
      A specific feature in the ./internal folder (e.g. forum, like, bookmark, ai, affiliate). The
      agent asks the user to input the feature name, then runs a full cross-stack audit (backend +
      frontend across all 5 apps + JEOS spec) for that feature, categorizes bugs by class, and saves
      findings to ./.jeos/audit/<feature_name>-audit.md.
  - label: "Fix feature audit findings"      (option 10)
      Apply fixes to the bugs recorded in ./.jeos/audit/<feature_name>-audit.md (produced by Option
      9). The agent asks for the feature name, reads the audit file, and fixes each open bug using
      the standard japa-bugs-hunters workflow (Options 1–8 fix rules). Every successfully fixed bug
      is marked **already implemented** in the audit file so it is never re-fixed.
```

After the user answers, read `Prompts/japa-bugs-hunters.md` in full, then execute the matching
workflow below. **Never skip reading the Guide** — it contains the section-number references you must
cite in fixes and reports.

**If the user picks Option 9:** immediately ask them to input the feature name (a folder under
`./internal`, e.g. `forum`, `like`, `bookmark`, `ai`, `affiliate`). Validate that
`./internal/<feature_name>` exists before proceeding; if it does not, list the available
`./internal/*` subdirectories and ask the user to pick one. Then execute the **Option 9 — Feature
Audit** workflow below.

**If the user picks Option 10:** immediately ask them to input the feature name. Validate that
`./.jeos/audit/<feature_name>-audit.md` exists; if it does not, list every file in `./.jeos/audit/`
and ask the user to pick one (or suggest running Option 9 first to produce the audit). Then execute
the **Option 10 — Fix Feature Audit Findings** workflow below.

**Shortcut triggers:**

Any of these skip the menu entirely. The phrase is case-insensitive and the quotes are optional
(e.g. `/japa-bugs-hunters "audit forum"`, `/japa-bugs-hunters fix /dashboard/overview`). The verb
`audit` vs `fix` only changes the workflow for **features** (Option 9 = audit-only, Option 10 =
fix-only); for every other target both verbs trigger that target's combined audit+fix workflow.

**Routing rules** (evaluate in order, first match wins):

1. **Feature audit/fix** — `/japa-bugs-hunters "audit [feature_name]"` or
   `/japa-bugs-hunters "fix [feature_name]"` where `[feature_name]` is a folder under `./internal`
   (e.g. `forum`, `like`, `bookmark`, `ai`, `affiliate`).
   - `audit [feature_name]` → **Option 9 — Feature Audit** (read-only, writes
     `./.jeos/audit/<feature_name>-audit.md`). Validate `./internal/<feature_name>` exists; if not,
     list `./internal/*` subdirectories and ask the user to pick one.
   - `fix [feature_name]` → **Option 10 — Fix Feature Audit Findings**. Validate
     `./.jeos/audit/<feature_name>-audit.md` exists; if not, list every file in `./.jeos/audit/` and
     offer to run Option 9 first.
2. **Backend API / route** — `/japa-bugs-hunters "audit /api/v1/<route>"` or
   `/japa-bugs-hunters "fix /api/v1/<route>"` (any target starting with `/api/v1/`, or a handler
   identifier like `forum.CreatePost`) → **Option 2 — Backend API / Route**.
3. **Frontend URL / page** — `/japa-bugs-hunters "audit <url-or-path>"` or
   `/japa-bugs-hunters "fix <url-or-path>"` where the target is a URL
   (`http://localhost:3000/...`, `https://admin.japaconnect.com/...`) or a frontend path
   (`/dashboard/overview`, `/forum/threads`, `/settings/profile`) → **Option 1 — Frontend URL / Page**.
   Resolve the app from the URL/path per Option 1's app-resolution rules.
4. **Whole surface** — `/japa-bugs-hunters "audit <surface>"` or
   `/japa-bugs-hunters "fix <surface>"` where `<surface>` is one of the surface keywords:
   - `backend`  → **Option 3 — Backend (whole)**
   - `main`     → **Option 4 — Main app**
   - `control`  → **Option 5 — Control panel**
   - `admin`    → **Option 6 — Admin panel**
   - `docs`     → **Option 7 — Docs site**
   - `learn`    → **Option 8 — Learn platform**

**Ambiguity fallback:** if a target could match multiple rules (e.g. a single word that is neither a
known `./internal/` feature nor a surface keyword), ask the user to clarify with the menu. When the
verb is omitted entirely (e.g. `/japa-bugs-hunters forum`), treat it as `audit` for features and as
the combined audit+fix for all other targets.

---

## Option 1 — Frontend URL / Page

### Trigger

- User picks **Option 1** from the menu, OR
- User invokes `/japa-bugs-hunters "audit <url-or-path>"` or `/japa-bugs-hunters "fix <url-or-path>"`
  with a URL or frontend path (see routing rule 3).

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

### Trigger

- User picks **Option 2** from the menu, OR
- User invokes `/japa-bugs-hunters "audit /api/v1/<route>"` or `/japa-bugs-hunters "fix /api/v1/<route>"`
  with a backend route or handler identifier (see routing rule 2).

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

### Trigger

- User picks **Option 3–8** from the menu, OR
- User invokes `/japa-bugs-hunters "audit <surface>"` or `/japa-bugs-hunters "fix <surface>"` with a
  surface keyword (`backend`, `main`, `control`, `admin`, `docs`, `learn`) — see routing rule 4.

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

## Option 9 — Feature Audit (from ./internal)

This is a **read-only audit + findings report** workflow. Unlike Options 1–8, Option 9 does **not**
auto-fix bugs — it produces a categorized findings file in `./.jeos/audit/` for the user to triage.
The user supplies `[feature_name]` = any folder under `./internal` (e.g. `forum`, `like`, `bookmark`,
`ai`, `affiliate`).

### Trigger

- User picks **Option 9** from the menu, OR
- User invokes `/japa-bugs-hunters "audit [feature_name]"` (case-insensitive, quotes optional).

Run the following prompt against the supplied `[feature_name]`, then save the output to
`./.jeos/audit/<feature_name>-audit.md` (create the `.jeos/audit/` folder if it does not exist):

> There are multiple irregularities in the [feature_name] features, thoroughly review and audit the
> [feature_name] feature for bugs, business logics, backend communication, UI Logics, UX Logics, and
> all its intended functionalities according to JEOS Specs. Save your findings to a file in
> `./.jeos/audit` folder.
>
> a. Map backend [feature_name] feature (handlers, services, stores, migrations, RBAC, Valkey, Asynq,
>    GDPR).
> b. Map frontend [feature_name] feature across all 5 apps (components, pages, API calls, UI/UX, i18n).
> c. Map JEOS spec for [feature_name] feature (sections, routes catalog, permissions, Valkey, Asynq,
>    settings, ADRs).
> d. Cross-check implementation against JEOS spec; identify conformance gaps.
> e. Categorize bugs by class (business logic, backend comms, UI/UX, RBAC, security, performance,
>    GDPR).

### Execution Steps

1. **Validate the feature name.** Confirm `./internal/<feature_name>` exists. If not, list
   `./internal/*` subdirectories and ask the user to pick a valid one.
2. **Read the Guide** (`Prompts/japa-bugs-hunters.md`) and the relevant `.jeos/sections/*` files for
   the feature (search by feature name). Also read `conventions/api-routes-catalog.md`,
   `permission-key-naming.md`, `valkey-keys-catalog.md`, `asynq-tasks.md`, `system-settings-catalog.md`.
3. **(a) Map backend** — list every handler, service, store, sqlc query, migration, RBAC node,
   Valkey key, Asynq task, and GDPR erasure entry that belongs to the feature. Use `grep`/`glob` over
   `./internal/<feature_name>`, `./internal/db/queries/`, `./db/migrations/`, `./internal/rbac/`,
   `./internal/valkey/`, `./internal/asynq/`, `./internal/gdpr/`.
4. **(b) Map frontend** — for each of the 5 apps (`apps/web`, `apps/admin`, `apps/control`,
   `apps/learn`, `apps/docs`), list components, pages/routes, API calls (`/api/v1/<feature>/*`),
   UI/UX flows, and i18n keys referencing the feature.
5. **(c) Map JEOS spec** — list every section, route catalog entry, permission key, Valkey key,
   Asynq task, system setting, and ADR that governs the feature.
6. **(d) Cross-check** — for each spec item, mark ✅ (implemented & conformant) / ⚠️ (implemented but
   non-conformant) / ❌ (missing). Record the conformance gap with file:line citations.
7. **(e) Categorize bugs** by class: `business-logic`, `backend-comms`, `ui-ux`, `rbac`, `security`,
   `performance`, `gdpr`. For each bug include: id, class, severity (critical/high/medium/low),
   title, file:line, description, spec reference, suggested fix.
8. **Save findings** to `./.jeos/audit/<feature_name>-audit.md` using the template below.
9. **Do not apply fixes** unless the user explicitly asks. Option 9 is audit-only; the user can later
   trigger Options 1–8 to fix specific bugs surfaced here.

### Findings File Template — `./.jeos/audit/<feature_name>-audit.md`

```markdown
# Feature Audit — <feature_name>

- Date: <ISO date>
- Auditor: japa-bugs-hunters (Option 9)
- Feature path: ./internal/<feature_name>

## 1. Backend Map
- Handlers: <list with file:line>
- Services: <list>
- Stores / sqlc queries: <list>
- Migrations: <list>
- RBAC nodes: <list>
- Valkey keys: <list>
- Asynq tasks: <list>
- GDPR erasure coverage: <tables covered / missing>

## 2. Frontend Map (all 5 apps)
### apps/web (main)
- Components: <list> · Pages: <list> · API calls: <list> · i18n keys: <list>
### apps/admin
- ...
### apps/control
- ...
### apps/learn
- ...
### apps/docs
- ...

## 3. JEOS Spec Map
- Sections: <list>
- Routes catalog entries: <list>
- Permission keys: <list>
- Valkey keys: <list>
- Asynq tasks: <list>
- System settings: <list>
- ADRs: <list>

## 4. Conformance Cross-Check
| Spec item | Status | Implementation | Gap |
| --- | --- | --- | --- |
| ... | ✅/⚠️/❌ | <file:line or "missing"> | <description> |

## 5. Bugs by Class
### business-logic
- BUG-001 [severity] — <title> — <file:line> — <description> — spec ref — suggested fix
### backend-comms
- ...
### ui-ux
- ...
### rbac
- ...
### security
- ...
### performance
- ...
### gdpr
- ...

## 6. Summary
- Total bugs: <n> (critical <n>, high <n>, medium <n>, low <n>)
- Conformance: <n> ✅ / <n> ⚠️ / <n> ❌
- Top priorities: <3-5 bullet list>
```

## Option 10 — Fix Feature Audit Findings

This workflow consumes the audit file produced by Option 9 (`./.jeos/audit/<feature_name>-audit.md`)
and applies fixes to every open bug entry. It reuses the standard japa-bugs-hunters fix rules
(Options 1–8 + Cross-Cutting Fix Rules) but is driven by the audit's bug list instead of a live
page/route scan.

### Trigger

- User picks **Option 10** from the menu, OR
- User invokes `/japa-bugs-hunters "fix [feature_name]"` (case-insensitive, quotes optional).

### Execution Steps

1. **Resolve the feature name.**
   - From the shortcut trigger: parse `[feature_name]` out of the user's input.
   - From the menu: ask the user to input the feature name.
2. **Locate the audit file** at `./.jeos/audit/<feature_name>-audit.md`.
   - If it does not exist: list every file in `./.jeos/audit/` and ask the user to pick one, or offer
     to run **Option 9** first to produce the audit. Do not proceed without an audit file.
3. **Read the audit file in full** and parse every bug entry under `## 5. Bugs by Class`. Each bug
   line follows the template:
   `- BUG-### [severity] — <title> — <file:line> — <description> — spec ref — suggested fix`
   - **Skip any bug line already prefixed with `✅ ALREADY IMPLEMENTED`** (see marking rule below)
     — those are previously fixed bugs and must never be re-fixed.
4. **Read the Guide** (`Prompts/japa-bugs-hunters.md`) in full before applying any fix — the section
   numbers cited in the audit's `spec ref` field map to Guide sections.
5. **Fix bugs in priority order:** `critical` → `high` → `medium` → `low`. Within a severity tier,
   fix in bug-id order (`BUG-001`, `BUG-002`, ...). Use a `todo_write` list to track every open bug.
6. **For each bug**, apply the matching fix workflow from Options 1–8 based on its class:
   - `business-logic`, `backend-comms`, `rbac`, `security`, `performance`, `gdpr` → use the
     **Option 2 (Backend API / Route)** workflow against the bug's `file:line`.
   - `ui-ux` → use the **Option 1 (Frontend URL / Page)** workflow against the bug's `file:line`
     (resolve the app from the path: `apps/web`→main, `apps/admin`→admin, `apps/control`→control,
     `apps/learn`→learn, `apps/docs`→docs). Launch Playwright to verify the fix when the bug is
     visual/interactive.
   - All fixes must obey the **Cross-Cutting Fix Rules** (Golden Rules, RBAC, multi-tenancy, custom
     fields) and the standard Report Format.
7. **Verify each fix** before marking it done:
   - Backend: `go build ./...` (and `go test ./internal/<feature_name>/...` if tests exist), plus
     `gosec` for security-class bugs.
   - Frontend: `svelte-check` for the relevant app, plus a Playwright pass for `ui-ux` bugs.
   - A fix is "successful" only when the relevant verifier passes and the original bug symptom is gone.
8. **Mark the bug line in the audit file** immediately after a successful fix. Use the `edit` tool to
   prefix the bug line with `✅ ALREADY IMPLEMENTED — ` and append ` — fixed <ISO date>` at the end.
   The marked line must look exactly like:

   ```
   - ✅ ALREADY IMPLEMENTED — BUG-### [severity] — <title> — <file:line> — <description> — spec ref — suggested fix — fixed <ISO date>
   ```

   **Marking rules (critical):**
   - Only mark a bug **after** its fix is verified — never speculatively.
   - Never delete or rewrite the original bug line; only prepend `✅ ALREADY IMPLEMENTED — ` and
     append ` — fixed <ISO date>`. The original text must remain intact for traceability.
   - If a fix fails verification, leave the bug line unmarked and record the failure reason in the
     final report. Do **not** mark it `ALREADY IMPLEMENTED`.
   - Never re-fix a line already prefixed with `✅ ALREADY IMPLEMENTED`. If the user re-runs Option 10,
     skip all marked lines and only process unmarked bug entries.
9. **Update the audit file's `## 6. Summary` section** at the end of the run with the new counts:
   - `Fixed this run: <n>`
   - `Already implemented (prior runs): <n>`
   - `Remaining open: <n>`
   Keep the original totals line intact for history; append the new lines below it.
10. **Produce the standard Bug Hunt Report** (see Report Format) at the end of the run, scoped to the
    feature and listing every file changed, every backend addition (routes, migrations, RBAC nodes,
    Valkey keys, Asynq tasks), and the verification results.

### Re-run Safety

Because every fixed bug is marked `✅ ALREADY IMPLEMENTED` in place, Option 10 is **idempotent**:
re-running it (or re-triggering `/japa-bugs-hunters "fix [feature_name]"`) will only process bugs
that are still open. This prevents duplicate work and avoids confusion when the user runs the fix
workflow multiple times or resumes after an interruption.

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
