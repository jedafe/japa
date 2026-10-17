---
name: jc-tools-audit
description: Use when auditing or reviewing the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japa.to). Covers host-detection correctness, shell mode rendering, API-Readiness Gate compliance, Rule 16 (no hardcoded data), and SSR/CSR boundary violations.
---

# JapaConnect Tools Surface — Audit Skill

Audit checklist for `apps/tools/` (§78, ADR-086). Run this when reviewing a PR, onboarding to the surface, or verifying a new tool page.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japa.to` (configurable via `system_settings`). Sixth SvelteKit app in the monorepo. Same Go backend, same `/api/v1`, **no community navigation**.

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

### 11. V2 Functional Tools — Geo-Aware Tool Pattern (§§90–97, §§99–101)
- [ ] All v2 functional tool API calls include `?dest=` filter param (destination country is primary data axis)
- [ ] No hardcoded nationality in any v2 tool page — destination selector is the first UI input
- [ ] `destination_country_id` present in API response schemas for v2 tool content
- [ ] Destination selector renders before any tool-specific content on every v2 tool page
- [ ] Feature-flag middleware returns **404 (not 403)** when a v2 tool is disabled (ADR-042) — verify the Tools app handles 404 gracefully (hides tool card, shows "coming soon")
- [ ] Each of the 12 new functional tools has a registry entry in `src/lib/tools/registry.ts` (slug, route, icon, accent color)
- [ ] Each v2 functional tool has `subdomain`/`custom_domain` system_settings keys configured (14 tool-domain keys total)
- [ ] Host detection in `hooks.server.ts` recognizes all 12 new tool slugs

**V2 functional tools to verify:**
- Visa Route Comparison (§90, `visa-route`) · Embassy Alerter (§91, `embassy-alerter`) · Arrival Checklist (§92, `arrival-checklist`) · Processing Time Tracker (§93, `processing-tracker`) · Credential Checker (§94, `credential-checker`) · CV Formatter (§95, `cv-formatter`) · Housing Guide (§96, `housing-guide`) · Attestation Guide (§97, `attestation-guide`) · Language Test Hub (§99, `language-test`) · Tax Calculator (§100, `tax-calculator`) · Community Locator (§101, `community-locator`)

### 12. ADR-088 — AI Content Generation Columns
- [ ] All admin-managed v2 tool content tables carry `ai_generated` (boolean), `ai_model` (varchar), `hitl_reviewed_by` (uuid FK), `ai_generated_at` (timestamp) columns
- [ ] `is_japabot_indexed` computed column present for RAG-retrievable content
- [ ] HITL review is mandatory before publish — content with `hitl_reviewed_by IS NULL` must not appear in public tool pages
- [ ] AI content generation goes through the §20 AI Gateway (no direct LLM calls from the Tools app)
- [ ] No unreviewed AI-generated content visible to end users

### 13. Tool-Domain System Settings
- [ ] Each v2 functional tool has `{slug}.subdomain` and `{slug}.custom_domain` settings in `system_settings` (14 tool-domain keys for §§79–101)
- [ ] Settings are exposed in the Control Panel system settings page with appropriate form fields
- [ ] Host detection reads these settings per-request (not cached at module level)

## Common Defects to Flag

1. **User data in SSR load** — leaks one user's progress into cached responses
2. **Hardcoded option lists** — violates Rule 16; must come from API selector
3. **Missing registration prompt** — anonymous tick/save silently fails instead of prompting
4. **Cross-tool nav in single-tool shell** — breaks the standalone tool UX
5. **`$env/static/public` for optional vars** — build fails if env var not set; use `$env/dynamic/public`
6. **`@lucide/svelte` instead of `lucide-svelte`** — wrong package, build fails
7. **`Gridset` import from LayerChart** — not exported in 2.0.1; use `LineChart` compound component
8. **Missing `?dest=` filter on v2 tool API calls** — geo-aware pattern violation; all v2 tool endpoints require destination country
9. **Hardcoded nationality in v2 tool page** — geo-aware pattern violation; destination selector must be first input, no nationality assumption
10. **Feature-flag returning 403 instead of 404** — ADR-042 violation; disabled features must return 404 to avoid leaking endpoint existence
11. **Missing ADR-088 AI generation columns** — v2 tool content tables must carry `ai_generated`/`ai_model`/`hitl_reviewed_by`/`ai_generated_at`
12. **HITL review bypassed** — unreviewed AI-generated content visible to users; `hitl_reviewed_by IS NULL` content must not appear publicly
13. **Missing tool-domain settings for new tools** — v2 functional tools without `subdomain`/`custom_domain` system_settings keys break host detection

## See Also

ADR-086, §78, `ROADMAP-TOOLS.md`, ADR-007, ADR-063, ADR-088 (AI content generation), ADR-042 (feature-flag 404), `.jeos/skills/geo-aware-tool-pattern.md`, §§90–97 §§99–101 (V2 functional tools), `jc-tools-develop`, `jc-tools-debug`, `jc-tools-refactor`.
