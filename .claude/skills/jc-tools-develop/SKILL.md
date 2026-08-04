---
name: jc-tools-develop
description: Use when developing the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japaconnect.com). Covers host-detection shell rendering, identity tool pages, and the standalone single-tool domain mode.
---

# JapaConnect Tools — Development Skill

Identity tools surface at `tools.japaconnect.com`. SSR (adapter-node + Bun). Sixth SvelteKit app in the monorepo (`apps/tools/`). Same Go backend, same `/api/v1`, **no community navigation** — tool-first UX.

## ⛔ API-READINESS GATE

Same as `jc-main-develop`: list endpoints → verify in `api-routes-catalog.md` → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock. Known tool routes (all verified live as of Backend Phase 44):

- **Visa Checklist (§70):** `GET /checklist/selector`, `GET /checklist/:guide_id`, `GET /checklist/:guide_id/progress`, `PATCH /checklist/:guide_id/progress/:item_key`, `DELETE /checklist/:guide_id/progress`
- **Readiness Assessment (§71):** `GET /assessment`, `POST /assessment/submit`, `GET /assessment/result/:id`, `GET /assessment/history`
- **Cost of Living (§72):** `GET /calculator/cost-of-living/cities`, `GET /calculator/cost-of-living/compare?city_ids=`
- **Scholarship Finder (§73):** `GET /scholarships` (with filter params), `GET /scholarships/:guide_id`, `GET /scholarships/saved`, `POST/PATCH/DELETE /scholarships/:guide_id/save`
- **Remittance Tracker (§74):** `GET /remittance/rates`, `GET /remittance/calculate`, `GET /remittance/history`, `GET /remittance/providers`, `GET /remittance/corridors`, `GET/POST/PATCH/DELETE /remittance/alerts`
- **JapaBot (§20, §69):** `POST /ai/chat`, `GET /ai/conversations`, `GET /ai/conversations/:id`

## Host Detection — The Core Mechanic (ADR-086, §78)

`hooks.server.ts` reads `system_settings` per-tool domain keys and sets `event.locals.toolSurface`:
- `'hub'` → full tools listing (default when host = `tools.japaconnect.com`)
- `'visa-checklist'` / `'readiness'` / `'cost-of-living'` / `'scholarships'` / `'remittance'` / `'japabot'` → single-tool shell

The host→surface map is built from `GET /site/settings/public` on each request. Each tool has two settings:
- `{tool}.subdomain` → `{subdomain}.japaconnect.com` (e.g. `remittance.japaconnect.com`)
- `{tool}.custom_domain` → exact host match (e.g. `japacalculator.com`)

Custom domains take precedence over subdomains. The `JC_TOOLS_HUB_HOSTS` env var adds extra hub hosts.

## Two Shell Modes

- **Hub shell** (`tools.japaconnect.com`): `HubHeader` (simplified — no community nav, just logo + theme toggle + auth links), tool cards grid, `Footer` (full).
- **Single-tool shell** (e.g. `remittance.japaconnect.com`): `SingleToolHeader` (tool name + icon with tool accent color, theme toggle, no cross-tool nav), full-screen tool content, `Footer` (minimal — "Powered by JapaConnect" link only).

Root `+layout.svelte` reads `data.toolSurface` and renders the appropriate shell. **Every tool route works in both shell modes** — the shell wrapping changes, the tool content does not.

## Tool Registry (`src/lib/tools/registry.ts`)

Structural metadata for each tool: slug, route, Lucide icon name, display name, tagline, description, accent color, `system_settings` key prefix. This is the tool **scaffold** (defined by §78), NOT domain data — all dynamic content comes from the API (Rule 16).

## Anonymous vs Authenticated State

Most tools are **anonymous-accessible** (browse, calculate, explore) with **registration prompts** for progress-saving:
- Visa Checklist: anonymous can see items; ticking prompts sign-in
- Readiness Assessment: anonymous can take the form; submitting requires auth
- Cost of Living: fully anonymous (no save state)
- Scholarship Finder: anonymous can browse; saving requires auth
- Remittance Tracker: anonymous can view rates; alerts require auth
- JapaBot: auth required (`ai.context.use` permission)

Registration prompts use a modal with links to `PUBLIC_MAIN_ORIGIN/signup` and `/login` (the Main app owns auth — Tools has no auth routes).

## SSR/CSR Boundary

SSR `+page.server.ts` load functions fetch **public** data (selector options, city lists, scholarship listings, rates, corridors). User-specific data (progress, saved items, alerts, conversations) is fetched SSR when authenticated, but mutations (tick, save, create alert) are **client-side** via `$lib/api/client` with CSRF token.

## LayerChart — Remittance Trend Chart (ADR-059)

Pinned `layerchart@2.0.1`. The remittance tracker uses `LineChart` for the 30-day rate history:
```svelte
<script lang="ts">import { LineChart } from 'layerchart';</script>
<LineChart {data} x="timestamp" y="rate" color="provider" />
```
Rules: colors via `var(--chart-*)` only (dark-mode safe) · fixed-height containers · data from `/api/v1/remittance/history` · per-chart skeleton · dates → `Date`.

## Tool Accent Colors

Each tool has an accent color (set via CSS `--tool-accent` on the shell header in single-tool mode). Used for: card hover borders, progress bars, gauge strokes, active states. Defined in the registry, applied via inline style.

## Stack

SvelteKit 2.57.x (SSR, adapter-node) · Bun 1.3.x · Tailwind CSS v4 · Bits UI · Lucide Svelte (`lucide-svelte@^1.0.1`) · LayerChart 2.0.1 (pinned) · env vars `JC_` prefix (public: `PUBLIC_JC_*`) · Umami analytics (`PUBLIC_JC_UMAMI_SITE_ID_TOOLS`, injected conditionally via `$env/dynamic/public`).

## Auth & Cross-App Links

Cookie auth (`domain=.japaconnect.com`) works for subdomains — authenticated users stay logged in across `japaconnect.com` and `tools.japaconnect.com`. Custom domains use OAuth flow back to `api.japaconnect.com` (see §78). Auth links (login, signup, profile) cross over to the Main app via `PUBLIC_MAIN_ORIGIN`.

## Deliverables Per Task

1. Gate output · 2. tool page built inside the appropriate shell (hub or single-tool) · 3. `+page.server.ts` (public data SSR, user data SSR when authed) · 4. `+page.svelte` (SSR content + CSR mutations, skeletons, registration prompts) · 5. components · 6. SEO metadata (title, description, OG, JSON-LD where applicable) · 7. types in `src/lib/types/index.ts`

## See Also

ADR-086 (multi-surface architecture), §78 (Tools Web Surface section), `ROADMAP-TOOLS.md`, ADR-007 (subdomain principle), ADR-063 (Japa University — host-detection precedent), `jc-tools-audit`, `jc-tools-debug`, `jc-tools-refactor`.
