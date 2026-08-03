---
name: japa-bugs-hunters
description: JapaConnect comprehensive bug hunter. On load it presents a menu — (1) a frontend URL or page name of any of the 6 SvelteKit apps (+ Flutter mobile), (2) a backend API route, (3-9) a whole surface (backend / main / control / admin / docs / learn / tools), (10) a feature name from ./internal (e.g. forum, like, bookmark, ai, affiliate, persona, rag, remittance, studio) for a full cross-stack audit against JEOS Specs with findings saved to ./.jeos/audit, or (11) apply fixes to a feature's audit findings in ./.jeos/audit/<feature_name>-audit.md. It hunts bugs from the JapaConnect Bug Hunter's Guide (.jeos/.ai/skills/japa-bugs-hunters.md) against the page/route/surface/feature, fixes frontend issues, and — when a bug needs backend work (missing route, unwired handler, incomplete CRUD, migration, RBAC, Valkey, Asynq, GDPR) — fixes the backend too until the target is 100% working. Covers v1 (sections 01–59) and v2 (sections 60–78, ADRs 061–086) features including AI personas, RAG, content studio, participation orchestration, remittance, identity tools, and the multi-surface tools/mobile architecture. Use when the user says "hunt bugs", "bug hunt", "audit and fix", "fix bugs on <page/route>", "audit feature <name>", "audit [feature_name]", "fix [feature_name]", "audit /api/v1/<route>", "fix /api/v1/<route>", "audit <url-or-path>", "fix <url-or-path>", "audit <surface>", "fix <surface>" (surface = backend/main/control/admin/docs/learn/tools), or triggers /japa-bugs-hunters.
---

# JapaConnect Bug Hunter

This skill turns the **JapaConnect Bug Hunter's Guide** (`.jeos/.ai/skills/japa-bugs-hunters.md`, the
canonical bug taxonomy) into an actionable, self-fixing workflow. The Guide is the taxonomy of bugs;
this skill is the engine that applies it.

**Authoritative business truth:** the `.jeos/` repo (JEOS v1.21.0) — `JEOS.md`, `sections/*` (78
numbered sections, 01–78, covering v1 features §§01–59 and v2 features §§60–78), `conventions/*`
(api-routes-catalog, permission-key-naming, system-settings-catalog, valkey-keys-catalog,
asynq-tasks, api-response-format, database-migration-naming, feature-flag-naming,
svelte-component-patterns), the **86 ADRs** (ADR-001–086), the 8 roadmap files (incl.
`ROADMAP-V2.md` planning index and `ROADMAP-TOOLS.md`), and `AGENTS.md` **17 Golden Rules**. Any
code that contradicts JEOS is a bug even if it compiles.

**v2 scope (JEOS v1.14.0–v1.21.0):** sections 60–78 and ADRs 061–086 add AI Persona System (§68,
ADR-068), JapaBot RAG Knowledge Layer (§69, ADR-069), Visa Checklist (§70), Readiness Assessment
(§71), Cost of Living (§72), Scholarship Finder (§73), Remittance Tracker (§74, ADR-075), AI
Content Studio (§75, ADR-076), Participation Orchestration (§76, ADR-077), Referral (§60,
ADR-078), Self-Serve Ads (§61, ADR-079), Newsletter (§62, ADR-080), Jobs Board (§63, ADR-081),
Gamification (§64, ADR-082), Content Import (§65, ADR-083), Advanced Analytics (§66, ADR-084),
Travel Search (§67, ADR-085), Flutter Mobile (§77, ADR-067/074), and Tools Web Surface (§78,
ADR-086). Three new architecture principles apply: **P9** (one backend, multiple client surfaces —
ADR-086), **P10** (synthetic activity always flagged internally via `is_bot`/`is_synthetic` —
ADR-068/077), **P11** (reuse over new infrastructure — ADR-063).

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
      A specific page on any of the 6 SvelteKit apps (main, admin, control, learn, docs, tools) or
      the Flutter mobile app. You'll supply a URL (or app+page name); the agent loads Playwright,
      tests the page against relevant bugs, fixes it, and fixes any backend the page needs.
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
  - label: "Tools web surface"        (option 9)
      Audit + fix apps/tools (port 3005, SSR, tools.japaconnect.com) against the Guide — the sixth
      SvelteKit app with host-detection hub/single-tool shell modes (ADR-086, §78).
  - label: "Feature name (from ./internal)"  (option 10)
      A specific feature in the ./internal folder (e.g. forum, like, bookmark, ai, affiliate,
      persona, rag, remittance, studio, newsletter, gamification). The agent asks the user to input
      the feature name, then runs a full cross-stack audit (backend + frontend across all 6 apps +
      mobile + JEOS spec) for that feature, categorizes bugs by class, and saves findings to
      ./.jeos/audit/<feature_name>-audit.md.
  - label: "Fix feature audit findings"      (option 11)
      Apply fixes to the bugs recorded in ./.jeos/audit/<feature_name>-audit.md (produced by Option
      10). The agent asks for the feature name, reads the audit file, and fixes each open bug using
      the standard japa-bugs-hunters workflow (Options 1–9 fix rules). Every successfully fixed bug
      is marked **already implemented** in the audit file so it is never re-fixed.
```

After the user answers, read `.jeos/.ai/skills/japa-bugs-hunters.md` in full, then execute the
matching workflow below. **Never skip reading the Guide** — it contains the section-number
references you must cite in fixes and reports.

**If the user picks Option 10:** immediately ask them to input the feature name (a folder under
`./internal`, e.g. `forum`, `like`, `bookmark`, `ai`, `affiliate`, `persona`, `rag`, `remittance`,
`studio`). Validate that `./internal/<feature_name>` exists before proceeding; if it does not, list
the available `./internal/*` subdirectories and ask the user to pick one. Then execute the
**Option 10 — Feature Audit** workflow below.

**If the user picks Option 11:** immediately ask them to input the feature name. Validate that
`./.jeos/audit/<feature_name>-audit.md` exists; if it does not, list every file in `./.jeos/audit/`
and ask the user to pick one (or suggest running Option 10 first to produce the audit). Then execute
the **Option 11 — Fix Feature Audit Findings** workflow below.

**Shortcut triggers:**

Any of these skip the menu entirely. The phrase is case-insensitive and the quotes are optional
(e.g. `/japa-bugs-hunters "audit forum"`, `/japa-bugs-hunters fix /dashboard/overview`). The verb
`audit` vs `fix` only changes the workflow for **features** (Option 10 = audit-only, Option 11 =
fix-only); for every other target both verbs trigger that target's combined audit+fix workflow.

**Routing rules** (evaluate in order, first match wins):

1. **Feature audit/fix** — `/japa-bugs-hunters "audit [feature_name]"` or
   `/japa-bugs-hunters "fix [feature_name]"` where `[feature_name]` is a folder under `./internal`
   (e.g. `forum`, `like`, `bookmark`, `ai`, `affiliate`, `persona`, `rag`, `remittance`, `studio`).
   - `audit [feature_name]` → **Option 10 — Feature Audit** (read-only, writes
     `./.jeos/audit/<feature_name>-audit.md`). Validate `./internal/<feature_name>` exists; if not,
     list `./internal/*` subdirectories and ask the user to pick one.
   - `fix [feature_name]` → **Option 11 — Fix Feature Audit Findings**. Validate
     `./.jeos/audit/<feature_name>-audit.md` exists; if not, list every file in `./.jeos/audit/` and
     offer to run Option 10 first.
2. **Backend API / route** — `/japa-bugs-hunters "audit /api/v1/<route>"` or
   `/japa-bugs-hunters "fix /api/v1/<route>"` (any target starting with `/api/v1/`, or a handler
   identifier like `forum.CreatePost`) → **Option 2 — Backend API / Route**.
3. **Frontend URL / page** — `/japa-bugs-hunters "audit <url-or-path>"` or
   `/japa-bugs-hunters "fix <url-or-path>"` where the target is a URL
   (`http://localhost:3000/...`, `https://admin.japaconnect.com/...`, `https://tools.japaconnect.com/...`)
   or a frontend path (`/dashboard/overview`, `/forum/threads`, `/settings/profile`) →
   **Option 1 — Frontend URL / Page**. Resolve the app from the URL/path per Option 1's
   app-resolution rules.
4. **Whole surface** — `/japa-bugs-hunters "audit <surface>"` or
   `/japa-bugs-hunters "fix <surface>"` where `<surface>` is one of the surface keywords:
   - `backend`  → **Option 3 — Backend (whole)**
   - `main`     → **Option 4 — Main app**
   - `control`  → **Option 5 — Control panel**
   - `admin`    → **Option 6 — Admin panel**
   - `docs`     → **Option 7 — Docs site**
   - `learn`    → **Option 8 — Learn platform**
   - `tools`    → **Option 9 — Tools web surface**

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
   - `localhost:3005` / `tools.*` → **tools** (SSR, ADR-086 hub/single-tool host-detection shell, §78)
   - Flutter `apps/mobile` (or a mobile screen name) → **mobile** (Dart, `japaconnect_core` client,
     §77/ADR-074 — Playwright does not apply; verify via `flutter analyze` + the typed API client)
2. **Launch Playwright** (use the `webapp-testing` skill / `playwright-cli`) for any SvelteKit app.
   Start the dev server for that app if not running (`bun dev` in the app dir). Open the URL, capture
   console errors, network failures (esp. 4xx/5xx on `/api/v1/*`), layout/visual screenshots at
   desktop + mobile widths, and hydration warnings. For the **tools** app, additionally verify the
   host-detection shell mode: hit the hub domain (all tools) and a per-tool domain/subdomain
   (single-tool shell, tool-specific branding, no cross-tool nav) per ADR-086/§78.
3. **Filter the Guide to the page.** Apply the `[FE]` and `[BOTH]` sections relevant to the page
   type: §1 (forms/nav/calc), §2 (UI/UX, dark mode, LayerChart), §3 (API calls, 401 loop, contract
   drift), §6 (Svelte 4 anti-patterns), §7 (i18n), §16 (SSR/hydration), §17 (admin/control SPA),
   §18 (i18n language packs), §21 (stub/missing UI), §23 (form data), §27 (Rule 16 hardcoded data),
   §36 (learning routes), §78 (tools host-detection shell). For v2 tool pages also check §70–§73
   (visa checklist / readiness / cost of living / scholarship finder UIs) and §74 (remittance UI).
4. **Fix frontend bugs** in place (Svelte 5 runes, Bits UI, LayerChart 2.0.1, `var(--chart-*)` theming,
   `$t()` i18n, no raw hex, no `export let`/`$:`/`on:`). For the tools app, respect the hub vs
   single-tool shell contract — a single-tool domain must NOT render cross-tool nav.
5. **If a bug needs the backend, fix the backend too — do not stop at the frontend.** This includes:
   - A route the page calls that **does not exist** → add it (see Option 2 workflow): pick the owning
     `sections/*` file (v1 §§01–59 or v2 §§60–78), add the route to `conventions/api-routes-catalog.md`,
     implement handler→service→store with sqlc, add any migration, wire RBAC node + CSRF, return the
     standard error envelope.
   - An **unwired/incomplete handler** (stub, no-op, missing CRUD leg) → implement it fully.
   - Missing **RBAC node / permission cache invalidation**, **Valkey key** (must be `jc:`-prefixed +
     tenant-scoped + cataloged), **Asynq task** (must be in asynq-tasks catalog — 95 tasks total),
     **system_settings** key (187 keys total), or **migration** → add/fix per JEOS.
   - **GDPR** gap if the page surfaces PII → cover the table in erasure.
   - **v2-specific:** missing `is_synthetic` tagging on persona activity (P10), missing high-stakes
     HITL gate on immigration/financial content, missing RAG embedding on publish, persona backing
     account not `is_bot`, content studio job not idempotent on `studio_job_id` → fix per the v2
     Mandatory Patterns (see Option 2 step 3).
6. Re-run Playwright on the page; iterate until **100% working** (no console errors, all network
   calls succeed, layout correct light+dark, i18n present, no hydration mismatch). For the tools app,
   verify both shell modes render correctly.

## Option 2 — Backend API / Route

### Trigger

- User picks **Option 2** from the menu, OR
- User invokes `/japa-bugs-hunters "audit /api/v1/<route>"` or `/japa-bugs-hunters "fix /api/v1/<route>"`
  with a backend route or handler identifier (see routing rule 2).

1. Locate the handler in `internal/` (match route → package → `server.go`/`handlers`). Confirm it is
   registered in chi and matches `conventions/api-routes-catalog.md` (~615 routes: ~475 v1 + ~140 v2).
2. Review the **full vertical slice**: route → handler → service → store (sqlc) → migration.
3. Apply the `[BE]` and `[BOTH]` sections: §1.3 (calc), §3 (API contract, error envelope, idempotency),
   §4 (queries, Valkey tenant-scoping, N+1, pgvector), §5 (SQLi, CSRF, RBAC, secrets), §6.2/6.3 (Go
   logic, nil deref), §8 (migrations, circular FK, sqlc regen), §9 (RBAC/multi-tenancy), §10 (Asynq
   catalog + idempotency + DLQ), §11 (WebSocket), §12 (Meilisearch tenant filter), §13 (payments/
   wallet ledger + `SET NX EX`), §14 (`content_hash`, soft-delete, Tiptap sanitize), §15 (GDPR erasure
   coverage), §19 (AI `ai.context.*` default-off, budget), §21 (stub/incomplete handler, missing CRUD
   leg), §22 (the 17 Golden Rules), §24 (field/data types), §25 (custom fields), §26 (RBAC CRUD +
   wiring). **v2 sections §§60–78** add feature-specific bug classes — apply the ones matching the
   route's feature (see the v2 Mandatory Patterns below).
4. **v2 Mandatory Patterns (JEOS v1.16.0–v1.21.0)** — check these when the route belongs to a v2
   feature, treat a violation as a bug:
   - **AI Personas (§68/ADR-068):** persona provisioning is transactional (account + byline +
     `ai_personas` config in one txn); `users.is_bot` is permanent on the backing account; the
     internal `is_bot`/`ai_personas` flag is always true even when public disclosure is off (P10);
     `is_synthetic` is set on `platform_events` for persona activity and excluded from real-metric
     materialized views; the high-stakes HITL gate (immigration/financial/grants/health) is
     unconditional and cannot be bypassed via config.
   - **RAG Knowledge Layer (§69/ADR-069):** `content_embeddings` uses **upsert** (not insert) on
     publish/edit via the existing `content:generate_embedding` task; TipTap JSON is extracted to
     plain text before embedding; source-tier (editorial vs community) is labelled; retrieval is
     top-k at `/ai/chat` time.
   - **AI Content Studio (§75/ADR-076):** `studio_jobs` is idempotent on `studio_job_id`; AI fills
     custom field values as structured output; all drafts enter the §22 `content_reviews` queue;
     `studio.auto_publish_types` may only auto-publish low-stakes types — immigration/financial guide
     categories permanently require review.
   - **Participation Orchestration (§76/ADR-077):** dispatched actions use the `is_bot` backing
     account's RBAC; a run-lock Valkey key prevents concurrent `persona:run_activity`; daily counters
     guard against budget overflow; `autonomous` vs `queued` autonomy is honoured; high-stakes gate
     overrides `autonomous`.
   - **Remittance Tracker (§74/ADR-075):** `remittance_providers.api_config` is encrypted at rest;
     `remittance_rates` history is append-only (never UPDATE/DELETE); the three tasks
     (`remittance:fetch_rates` / `check_alerts` / `prune_old_rates`) are registered in the catalog;
     does NOT touch §58 fixed credit-economy rates.
   - **Newsletter (§62/ADR-080):** unsubscribe tokens are single-use; bounce/complaint webhooks
     update subscriber status; marketing email uses the ADR-015 provider abstraction; GDPR
     one-click unsubscribe is honoured.
   - **Referral (§60/ADR-078):** first-click 30-day attribution; fraud controls per ADR-026; per-period
     cap enforced; Credits reward via §28 ledger (idempotent).
   - **Self-Serve Ads (§61/ADR-079):** verified-org gate; upfront credit payment via §17/§28; creative
     review via §22 queue; `ads:refund_campaign_credits` on rejection.
   - **Jobs Board (§63/ADR-081):** `job_listing` content type via §14; `jobs:expire_listings` daily
     task; on-platform apply via §06 forms.
   - **Gamification (§64/ADR-082):** evaluated inside `wallet:process_activity_event`; Valkey
     leaderboard; `gamification:expire_challenges` task; counter overflow guarded.
   - **Tools Web Surface (§78/ADR-086):** host detection reads per-tool `system_settings` domain keys;
     CORS `allowed_origins` extended per active domain; same binary serves hub + single-tool shells.
5. **Fix:** implement missing logic, complete unwired handlers, add the missing migration/sqlc query
   (edit `db/queries/`, run `make sqlc-gen` — never hand-edit `internal/db/`), add RBAC node + cache
   invalidation, add Valkey key to catalog, register Asynq task in catalog, enforce `system_settings`,
   return standard `{ "data": ... }` / `{ "error": { "code", "message" } }`.
6. Verify the route returns non-404/non-500 under correct auth and matches the section's contract.
   For v2 routes, also verify the v2 Mandatory Pattern above for that feature holds.

## Options 3–9 — Whole Surface Audit + Fix

### Trigger

- User picks **Option 3–9** from the menu, OR
- User invokes `/japa-bugs-hunters "audit <surface>"` or `/japa-bugs-hunters "fix <surface>"` with a
  surface keyword (`backend`, `main`, `control`, `admin`, `docs`, `learn`, `tools`) — see routing rule 4.

Map the surface to its bug sections, then audit file-by-file and fix:

- **Option 3 — Backend (whole):** all `[BE]`/`[BOTH]` sections above; prioritize §21 (stub/missing
  routes — every one of the ~615 catalogued routes must be reachable & non-stub), §22 (17 Golden
  Rules), §26 (RBAC node reconciliation — 297 nodes total), §9 (tenant scoping), §10 (Asynq catalog —
  95 tasks). Apply the **v2 catalog completeness gate**: all four catalogs (routes, settings, Valkey,
  Asynq) must reflect v2 code before a v2 phase is marked complete; verify the 20 new v2 tables
  (§§60–78); verify task idempotency per v2 task; verify the high-stakes gate cannot be bypassed via
  config; verify synthetic activity (`is_synthetic`) is excluded from real-metric materialized views.
  Use the `japa-backend-auditor` skill's route/node/catalog reconciliation approach (incl. its
  Section K — V2 Audit Checklist) as a sub-routine.
- **Option 4 — Main (apps/main):** §1, §2, §3, §6, §7, §16 (SSR/hydration), §17 (where shared), §18,
  §21, §27 (Rule 16), §36 (learning routes). Enforce ADR-058 shell, LayerChart 2.0.1, Bits UI,
  `$t()` i18n, no hardcoded data, JSON-LD/SEO. For v2 main phases (33–36: identity tools UI,
  community+social, growth+import), also check §§70–73 tool UIs and §74 remittance UI. Use
  `jc-main-audit` patterns.
- **Option 5 — Control (apps/control):** §2, §3, §6, §7, §16 (SPA), §17 (SPA specifics: auth guard
  loop, polling leak, Bits UI key handling, LayerChart `xPadding` not `xNice`), §21, §27. For v2
  control phases, verify settings forms for the 62 new v2 `system_settings` keys. Use
  `jc-control-audit` patterns.
- **Option 6 — Admin (apps/admin):** §2, §3, §6, §7, §16 (SPA), §17, §21, §23 (forms), §25 (custom
  fields), §26 (RBAC wiring vs sidebar nav map), §27. For v2 admin phases (20–23: persona management
  + studio, newsletter + jobs, gamification + analytics + commerce), also check §§68/75/62/63/64/66
  admin surfaces. Use `jc-admin-audit` patterns.
- **Option 7 — Docs (apps/docs):** §1, §2, §3, §6, §7, §16 (SSR, ADR-060 nav-tree/TOC), §18, §21, §27.
  Verify OpenAPI spec currency against the ~615-route catalog (§47).
- **Option 8 — Learn (apps/learn):** §1, §2, §3, §6, §7, §16 (SSR, ADR-060 lesson-player exception),
  §18, §21, §27, §36 (learning routes). No new v2 learn pages per ADR-060.
- **Option 9 — Tools (apps/tools):** §1, §2, §3, §6, §7, §16 (SSR), §18, §21, §27, §78 (host-detection
  hub/single-tool shell, ADR-086). Verify `hooks.server.ts` host detection reads per-tool
  `system_settings` domain keys (14 tool-domain settings); verify hub shell renders all tools and
  single-tool shell renders tool-specific branding with NO cross-tool nav; verify CORS
  `allowed_origins` covers active domains; verify `japaconnect_core` import where used. Use
  `jc-tools-audit` patterns.

For each surface, produce the structured report (see below) and apply fixes until clean. Re-run the
dev server / `svelte-check` / `go build ./...` / `gosec` as appropriate to verify.

## Option 10 — Feature Audit (from ./internal)

This is a **read-only audit + findings report** workflow. Unlike Options 1–9, Option 10 does **not**
auto-fix bugs — it produces a categorized findings file in `./.jeos/audit/` for the user to triage.
The user supplies `[feature_name]` = any folder under `./internal` (e.g. `forum`, `like`, `bookmark`,
`ai`, `affiliate`, `persona`, `rag`, `remittance`, `studio`, `newsletter`, `gamification`).

### Trigger

- User picks **Option 10** from the menu, OR
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
> b. Map frontend [feature_name] feature across all 6 SvelteKit apps + Flutter mobile (components,
>    pages, API calls, UI/UX, i18n).
> c. Map JEOS spec for [feature_name] feature (sections, routes catalog, permissions, Valkey, Asynq,
>    settings, ADRs).
> d. Cross-check implementation against JEOS spec; identify conformance gaps.
> e. Categorize bugs by class (business logic, backend comms, UI/UX, RBAC, security, performance,
>    GDPR).

### Execution Steps

1. **Validate the feature name.** Confirm `./internal/<feature_name>` exists. If not, list
   `./internal/*` subdirectories and ask the user to pick a valid one.
2. **Read the Guide** (`.jeos/.ai/skills/japa-bugs-hunters.md`) and the relevant `.jeos/sections/*`
   files for the feature (search by feature name — v1 §§01–59 and v2 §§60–78). Also read
   `conventions/api-routes-catalog.md`, `permission-key-naming.md`, `valkey-keys-catalog.md`,
   `asynq-tasks.md`, `system-settings-catalog.md`.
3. **(a) Map backend** — list every handler, service, store, sqlc query, migration, RBAC node,
   Valkey key, Asynq task, and GDPR erasure entry that belongs to the feature. Use `grep`/`glob` over
   `./internal/<feature_name>`, `./internal/db/queries/`, `./db/migrations/`, `./internal/rbac/`,
   `./internal/valkey/`, `./internal/asynq/`, `./internal/gdpr/`. For v2 features, also check the v2
   Mandatory Patterns (Option 2 step 4) and `is_synthetic`/high-stakes-gate conformance.
4. **(b) Map frontend** — for each of the 6 SvelteKit apps (`apps/main`, `apps/admin`,
   `apps/control`, `apps/learn`, `apps/docs`, `apps/tools`) plus the Flutter `apps/mobile` where
   relevant, list components, pages/routes, API calls (`/api/v1/<feature>/*`), UI/UX flows, and i18n
   keys referencing the feature.
5. **(c) Map JEOS spec** — list every section (v1 §§01–59 or v2 §§60–78), route catalog entry,
   permission key, Valkey key, Asynq task, system setting, and ADR that governs the feature.
6. **(d) Cross-check** — for each spec item, mark ✅ (implemented & conformant) / ⚠️ (implemented but
   non-conformant) / ❌ (missing). Record the conformance gap with file:line citations.
7. **(e) Categorize bugs** by class: `business-logic`, `backend-comms`, `ui-ux`, `rbac`, `security`,
   `performance`, `gdpr`. For each bug include: id, class, severity (critical/high/medium/low),
   title, file:line, description, spec reference, suggested fix.
8. **Save findings** to `./.jeos/audit/<feature_name>-audit.md` using the template below.
9. **Do not apply fixes** unless the user explicitly asks. Option 10 is audit-only; the user can
   later trigger Options 1–9 or Option 11 to fix specific bugs surfaced here.

### Findings File Template — `./.jeos/audit/<feature_name>-audit.md`

```markdown
# Feature Audit — <feature_name>

- Date: <ISO date>
- Auditor: japa-bugs-hunters (Option 10)
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

## 2. Frontend Map (all 6 apps + mobile)
### apps/main
- Components: <list> · Pages: <list> · API calls: <list> · i18n keys: <list>
### apps/admin
- ...
### apps/control
- ...
### apps/learn
- ...
### apps/docs
- ...
### apps/tools
- ...
### apps/mobile (Flutter, if relevant)
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

## Option 11 — Fix Feature Audit Findings

This workflow consumes the audit file produced by Option 10 (`./.jeos/audit/<feature_name>-audit.md`)
and applies fixes to every open bug entry. It reuses the standard japa-bugs-hunters fix rules
(Options 1–9 + Cross-Cutting Fix Rules) but is driven by the audit's bug list instead of a live
page/route scan.

### Trigger

- User picks **Option 11** from the menu, OR
- User invokes `/japa-bugs-hunters "fix [feature_name]"` (case-insensitive, quotes optional).

### Execution Steps

1. **Resolve the feature name.**
   - From the shortcut trigger: parse `[feature_name]` out of the user's input.
   - From the menu: ask the user to input the feature name.
2. **Locate the audit file** at `./.jeos/audit/<feature_name>-audit.md`.
   - If it does not exist: list every file in `./.jeos/audit/` and ask the user to pick one, or offer
     to run **Option 10** first to produce the audit. Do not proceed without an audit file.
3. **Read the audit file in full** and parse every bug entry under `## 5. Bugs by Class`. Each bug
   line follows the template:
   `- BUG-### [severity] — <title> — <file:line> — <description> — spec ref — suggested fix`
   - **Skip any bug line already prefixed with `✅ ALREADY IMPLEMENTED`** (see marking rule below)
     — those are previously fixed bugs and must never be re-fixed.
4. **Read the Guide** (`.jeos/.ai/skills/japa-bugs-hunters.md`) in full before applying any fix — the section
   numbers cited in the audit's `spec ref` field map to Guide sections.
5. **Fix bugs in priority order:** `critical` → `high` → `medium` → `low`. Within a severity tier,
   fix in bug-id order (`BUG-001`, `BUG-002`, ...). Use a `todo_write` list to track every open bug.
6. **For each bug**, apply the matching fix workflow from Options 1–9 based on its class:
   - `business-logic`, `backend-comms`, `rbac`, `security`, `performance`, `gdpr` → use the
     **Option 2 (Backend API / Route)** workflow against the bug's `file:line`. For v2-feature bugs,
     also apply the v2 Mandatory Patterns (Option 2 step 4).
   - `ui-ux` → use the **Option 1 (Frontend URL / Page)** workflow against the bug's `file:line`
     (resolve the app from the path: `apps/main`→main, `apps/admin`→admin, `apps/control`→control,
     `apps/learn`→learn, `apps/docs`→docs, `apps/tools`→tools, `apps/mobile`→mobile). Launch
     Playwright to verify the fix when the bug is visual/interactive (skip for `apps/mobile` — use
     `flutter analyze` instead).
   - All fixes must obey the **Cross-Cutting Fix Rules** (Golden Rules, RBAC, multi-tenancy, custom
     fields, v2 patterns) and the standard Report Format.
7. **Verify each fix** before marking it done:
   - Backend: `go build ./...` (and `go test ./internal/<feature_name>/...` if tests exist), plus
     `gosec` for security-class bugs.
   - Frontend: `svelte-check` for the relevant app, plus a Playwright pass for `ui-ux` bugs. For
     `apps/mobile` use `flutter analyze`.
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
   - Never re-fix a line already prefixed with `✅ ALREADY IMPLEMENTED`. If the user re-runs Option 11,
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

Because every fixed bug is marked `✅ ALREADY IMPLEMENTED` in place, Option 11 is **idempotent**:
re-running it (or re-triggering `/japa-bugs-hunters "fix [feature_name]"`) will only process bugs
that are still open. This prevents duplicate work and avoids confusion when the user runs the fix
workflow multiple times or resumes after an interruption.

---

## Cross-Cutting Fix Rules (must hold for every fix)

- **Golden Rules (`AGENTS.md`):** section is source of truth (R1); never hand-edit `internal/db/` (R2);
  config from `JC_*`/`system_settings`, never hardcoded (R3); Valkey keys `jc:`-prefixed + cataloged
  (R4); standard error envelope (R5); invalidate `jc:perm:{user_id}` on group/role/mod/ban changes (R6);
  Asynq task name in catalog (R7); fan-out fire-and-forget (R8); `SET NX EX` idempotency on financial
  endpoints (R9); impersonation writes `audit_logs` (R10); soft-delete only for user content (R11);
  compute `content_hash` before every `contents` save (R12); circular-FK migration ordering (R13);
  read all referenced sections (R14); all ~615 routes return non-404 under auth (R15); frontend
  API-only, no hardcoded/mock data (R16); no AI authorship attribution (R17).
- **RBAC:** every `hasPermission()` node must exist in the `permissions` seed using `_own`/`_any`
  scoping; `super_admin` holds all nodes (297 nodes total: 249 v1 + 48 v2); `members` holds exactly
  the 45 member-level nodes.
- **Multi-tenancy:** every query, Valkey key, and Meilisearch filter carries `tenant_id`.
- **Custom fields:** correct `field_type`, `options` jsonb shape, RBAC-per-field scoping, valid
  `entity_class` assignment.
- **v2 patterns (JEOS v1.16.0–v1.21.0):** synthetic activity is always flagged internally via
  `is_bot`/`is_synthetic` and excluded from real-metric materialized views (P10); the high-stakes
  HITL gate (immigration/financial/grants/health) is unconditional and cannot be bypassed via config;
  persona backing accounts are permanently `is_bot`; RAG embeddings use upsert on publish/edit;
  content studio jobs are idempotent on `studio_job_id`; remittance `api_config` is encrypted and
  rate history is append-only; newsletter unsubscribe tokens are single-use; one backend serves all
  client surfaces — no per-surface backend duplication (P9); reuse existing infrastructure over
  building new (P11).

## Report Format

End every run with:

```
## Bug Hunt Report — <target>
- Mode: <option 1-11> · App/Surface: <...> · Route(s): <...>
- Bugs found: <n>  (Frontend: <n> · Backend: <n>)
- Fixed: <n>  · Remaining: <n> (with reason if any)
- Backend changes: routes added <list>, migrations <list>, RBAC nodes <list>, Valkey keys <list>,
  Asynq tasks <list>
- Files changed: <list>
- Verification: <Playwright pass / go build / svelte-check / flutter analyze / gosec result>
- Checklist A (JEOS Conformance) and B (Feature-completeness) from the Guide: ✅/❌ per item
- v2 conformance (if a v2 feature): synthetic-activity flagging ✅/❌, high-stakes gate ✅/❌,
  catalog completeness gate ✅/❌
```

## Reference Files

- `.jeos/.ai/skills/japa-bugs-hunters.md` — the full bug taxonomy (read first, every run).
- `.jeos/JEOS.md` — index of all 78 sections, 86 ADRs, 10 conventions, 8 roadmaps.
- `.jeos/AGENTS.md` — the 17 Golden Rules.
- `.jeos/conventions/api-routes-catalog.md` — ~615 routes (~475 v1 + ~140 v2; existence/contract check).
- `.jeos/conventions/permission-key-naming.md` (297 RBAC nodes), `system-settings-catalog.md`
  (187 keys), `valkey-keys-catalog.md` (69 patterns), `asynq-tasks.md` (95 tasks),
  `api-response-format.md`, `database-migration-naming.md`, `feature-flag-naming.md`,
  `svelte-component-patterns.md`.
- `.jeos/sections/03-rbac.md`, `sections/06-custom-field-builder-form-builder.md` (v1); v2 sections
  `sections/60-referral-system.md` through `sections/78-tools-web-surface.md`.
- `.jeos/adr/ADR-058-main-app-shell.md`, `ADR-060-learn-docs-shell-profile-hub.md`,
  `ADR-068-ai-persona-system.md`, `ADR-069-japabot-rag-knowledge-layer.md`,
  `ADR-076-ai-content-studio.md`, `ADR-077-participation-orchestration-engine.md`,
  `ADR-086-multi-surface-client-architecture.md`.
- `.jeos/roadmaps/ROADMAP-V2.md` (v2 planning index), `ROADMAP-TOOLS.md` (tools surface).
- `.jeos/.ai/skills/backend/` and `.jeos/.ai/skills/frontend/` — the jc-* develop/audit/debug/refactor
  skills per surface (use as sub-routines per the surface options above).
