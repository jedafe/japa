---
name: jc-tools-audit
description: Use when auditing or reviewing the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japaconnect.com). Covers host-detection correctness, shell mode rendering, API-Readiness Gate compliance, Rule 16 (no hardcoded data), and SSR/CSR boundary violations.
---

# JapaConnect Tools Surface — Audit Skill

Audit checklist for `apps/tools/` (§78, ADR-086). Run this when reviewing a PR, onboarding to the surface, or verifying a new tool page.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japaconnect.com` (configurable via `system_settings`). Sixth SvelteKit app in the monorepo. Same Go backend, same `/api/v1`, **no community navigation**.

## Audit Checklist

### 1. Host Detection (`hooks.server.ts`)
- [ ] Reads `system_settings` per-tool domain keys on every request (not cached at module level)
- [ ] Sets `event.locals.toolSurface` to `'hub'` or a specific tool slug
- [ ] Custom domains take precedence over subdomains
- [ ] Falls back to `'hub'` when host matches no configured tool domain
- [ ] `JC_TOOLS_HUB_HOSTS` env var adds extra hub hosts (comma-separated)
- [ ] Theme, locale, session, CSRF token resolved consistently with other apps

### 2. Shell Mode Rendering (`+layout.svelte`)
- [ ] Hub shell: `HubHeader` (no community nav), tool cards grid, full `Footer`
- [ ] Single-tool shell: `SingleToolHeader` (tool name + accent color), full-screen content, minimal `Footer`
- [ ] Every tool route works in **both** shell modes (shell wrapping changes, tool content does not)
- [ ] `--tool-accent` CSS variable set on the shell in single-tool mode
- [ ] No cross-tool navigation in single-tool shell

### 3. API-Readiness Gate
- [ ] Every `+page.server.ts` load function only calls endpoints verified in `api-routes-catalog.md`
- [ ] No mocked or placeholder data (Rule 16)
- [ ] Tool routes used:
  - Visa Checklist: `/checklist/selector`, `/checklist/:guide_id`, `/checklist/:guide_id/progress/:item_key`
  - Readiness: `/assessment`, `/assessment/submit`, `/assessment/result/:id`, `/assessment/history`
  - Cost of Living: `/calculator/cost-of-living/cities`, `/calculator/cost-of-living/compare`
  - Scholarships: `/scholarships`, `/scholarships/:guide_id`, `/scholarships/saved`, `/scholarships/:guide_id/save`
  - Remittance: `/remittance/rates`, `/calculate`, `/history`, `/providers`, `/corridors`, `/alerts`
  - JapaBot: `/ai/chat`, `/ai/conversations`

### 4. SSR/CSR Boundary
- [ ] `+page.server.ts` fetches **public** data only (selector options, city lists, rates, corridors)
- [ ] User-specific data (progress, saved items, alerts, conversations) fetched SSR **only when authenticated**
- [ ] Mutations (tick, save, create alert, send chat) are client-side via `$lib/api/client` with CSRF token
- [ ] No user-specific data in cached SSR responses (would leak across users)

### 5. Anonymous vs Authenticated State
- [ ] Anonymous users can browse tools (see items, calculate, view rates)
- [ ] Registration prompts appear on first save/tick/submit action (not before)
- [ ] Registration modal links to `PUBLIC_MAIN_ORIGIN/signup` and `/login` (Main app owns auth)
- [ ] JapaBot requires auth (`ai.context.use` permission) — sign-in prompt shown to guests

### 6. Rule 16 (No Hardcoded Data)
- [ ] Tool registry (`src/lib/tools/registry.ts`) contains only structural scaffold metadata (slug, route, icon, name, accent color) — NOT domain data
- [ ] No hardcoded country lists, currency lists, visa types, city lists, scholarship lists, or rate data
- [ ] All dynamic content comes from the API
- [ ] Filter options (visa types, levels of study) come from the API selector endpoint

### 7. Rule 17 (No AI Attribution)
- [ ] No commit messages, PR descriptions, code comments, or docs attributing authorship to an AI
- [ ] No "Generated with Claude/Cursor/Devin" trailers
- [ ] No `// AI: added this` or `// TODO(agent)` comments

### 8. SEO & Metadata
- [ ] Every page has `<title>`, `<meta name="description">`, canonical URL, OG tags
- [ ] JSON-LD where applicable (hub page `WebSite`, scholarships `EducationalOccupationalProgram`)
- [ ] Shareable URLs via query params (visa checklist `guide_id`, cost of living `city_ids`, assessment `result`, scholarships filter params)

### 9. LayerChart Usage (Remittance)
- [ ] `layerchart@2.0.1` pinned exactly
- [ ] `LineChart` for 30-day rate history
- [ ] Colors via `var(--chart-*)` only (dark-mode safe)
- [ ] Fixed-height containers
- [ ] Per-chart skeleton during loading

### 10. Stack Compliance
- [ ] SvelteKit 2.57.x (SSR, adapter-node)
- [ ] Bun 1.3.x
- [ ] Tailwind CSS v4 (`@import 'tailwindcss'`, `@theme inline`)
- [ ] Bits UI (not raw headless UI)
- [ ] `lucide-svelte@^1.0.1` (not `@lucide/svelte`)
- [ ] Env vars `JC_` prefix (public: `PUBLIC_JC_*`)
- [ ] Umami analytics conditional via `$env/dynamic/public` (not `$env/static/public` — those vars are optional)

## Common Defects to Flag

1. **User data in SSR load** — leaks one user's progress into cached responses
2. **Hardcoded option lists** — violates Rule 16; must come from API selector
3. **Missing registration prompt** — anonymous tick/save silently fails instead of prompting
4. **Cross-tool nav in single-tool shell** — breaks the standalone tool UX
5. **`$env/static/public` for optional vars** — build fails if env var not set; use `$env/dynamic/public`
6. **`@lucide/svelte` instead of `lucide-svelte`** — wrong package, build fails
7. **`Gridset` import from LayerChart** — not exported in 2.0.1; use `LineChart` compound component

## See Also

ADR-086, §78, `ROADMAP-TOOLS.md`, ADR-007, ADR-063, `jc-tools-develop`, `jc-tools-debug`, `jc-tools-refactor`.
