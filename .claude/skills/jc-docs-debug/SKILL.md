---
name: jc-docs-debug
description: [Docs app, docs.japaconnect.com] Unified shell adapted for Docs (ADR-060) — nav-tree left nav, TOC right sidebar, playground full-width exception. Use when reviewing or debugging JapaConnect Docs app code (apps/docs, SSR). Diagnoses IntersectionObserver and keydown listener leaks, Cmd+K search issues, code block highlighting, and responsive three-column collapse.
---

# JapaConnect Docs — Review & Debugging Skill

Conventions and gates: `jc-docs-develop`. Run in order.

## 0. API-Readiness First
Data not loading → verify route before debugging (⚠️ alert if missing, pause). Sidebar empty? Check `/api/v1/docs/sections` responds — do NOT hardcode a fallback tree.

## 1. Observer/Listener Leaks (Docs' #1 Bug)
- IntersectionObserver in `$effect` MUST return `() => observer.disconnect()` — accumulation breaks anchor highlighting and leaks memory
- Cmd+K `window.addEventListener('keydown', ...)` MUST return removeEventListener cleanup — stacked listeners open multiple dialogs
- Any `ResizeObserver`/`setInterval` similarly cleaned up

## 2. Cmd+K Search
`<Combobox.Input>` auto-focused on dialog open · first focusable inside `<Dialog.Content>` · Escape closes without Combobox conflict · debounce cleanup (`return () => clearTimeout(id)`) · results from `/docs/search` not local filtering.

## 3. Code Blocks
Highlighting in `$effect` + browser guard + dynamic import (not in SSR bundle) · re-runs on route change (new page content) · copy reads `textContent` not innerHTML · "Copied!" tooltip resets after ~2s.

## 4. Sidebar
From `/api/v1/docs/sections` (Rule 16) · active page tracks `$page.url.pathname` · mobile drawer is `<Dialog.Root>` with focus trap, focus returns to hamburger on close · nav data cached (no re-fetch per internal navigation).

## 5. Responsive Collapse
Three→one column at breakpoints · right panel hidden ≤ tablet · no horizontal overflow at any width.

## 6. `{@html}` Safety
Only for backend TipTap-rendered docs content — flag any other instance.

## 7. Standard Checks
Apply `jc-main-debug` §§1–3, 5–7 (SSR/hydration, Runes, Bits UI, Tailwind, SEO, a11y). LayerChart checks (`jc-main-debug` §4) if usage-stat charts present.

## 8. Japa Academy & Geo API Docs (ADR-063/064)
- **Missing API reference pages:** if the new v1.11.0 routes (`/geo/me`, `/academy/*`, `/universities`, `/admin/universities`, `/me/payouts`) don't have API reference pages, that's a docs gap — flag it
- **`?geo=` param not documented:** feed/search/trending routes should document the `?geo=` default behavior (ContentCountry when absent, `?geo=all` for global)
- **`price_kobo` unit confusion:** if docs show `price_kobo` without specifying it's an integer in kobo (not naira), flag it
- **`is_published_to_academy` vs `is_published`:** if docs conflate these two flags, flag it — they're distinct (marketplace visibility vs platform publish)

## Output Protocol
1. Root cause · 2. Broken code · 3. Fixed code · 4. Secondary findings

## App Shell & Docs Drift (ADR-060)
- Docs uses the unified shell (ADR-058) adapted per ADR-060: the **left nav IS the audience-gated doc nav tree** (from `GET /api/v1/docs/nav`), collapsible (`ui.nav_collapsed` server-side, not localStorage); the **right sidebar IS the in-page TOC** + "was this helpful?" — flag social-widget sidebars or a hardcoded nav tree.
- API reference adds a right code-sample pane; the API Playground is the full-width tool exception — flag a playground crammed into the three-column shell.
- Audience gating is server-side only (403 → `/docs/access-denied`) — flag any client-side permission hiding.
- ⌘K search + TOC observers must clean up in `$effect` return.
- Any usage-stat charts use pinned `layerchart@2.0.1`, CSS-var colors, fixed height.
**V2 docs app:** No new pages in v2. Docs app requires stable API surface (OpenAPI spec from §47). The `japaconnect_core` Flutter package will use the same OpenAPI spec — ensure spec remains current when v2 backend phases land. **JEOS v1.22.0–v1.23.1 updates (§§79–102, ADRs 087–110):**
- **OpenAPI spec must cover the new ~344 v2 routes** (§§60–102) — the spec from §47 must be extended to include all v2 community feature routes (§§79–89, §98), functional tool routes (§§90–97, §§99–101), and mentorship routes (§102). Total platform route count is now ~819 (~475 v1 + ~344 v2).
- **New ADRs 087–110 need documentation pages** — each ADR should have a docs page in the architecture/ADRs section. Key ADRs: ADR-088 (universal AI content generation), ADR-042 (feature-flag 404 not 403), ADR-110 (mentorship matching).
- **New sections §§79–102 need API reference documentation** — 24 new sections covering 11 v2 community features, 12 V2 functional tools, and mentorship matching. Each section's endpoints need three-column API-docs variant pages with method badges and Request/Response/Example tabs.
- **Geo-aware tool pattern needs a docs page** — `.jeos/skills/geo-aware-tool-pattern.md` describes the destination-country-as-primary-axis pattern. This needs a public docs page explaining the `?dest=` filter, `destination_country_id` schema field, destination selector UX, and feature-flag 404 behavior.
- **ADR-088 AI content generation columns** need documentation: `ai_generated`, `ai_model`, `hitl_reviewed_by`, `ai_generated_at`, `is_japabot_indexed` — document these as standard columns on all v2 tool content tables, with HITL review mandatory before publish.
- 18 Golden Rules (R18: always use bun).

## V2 Feature Areas (Implement Only After Corresponding Backend Phase Is Complete)

| Feature | Backend phase | Key sections | Gates |
|---|---|---|---|
| JapaBot (RAG-grounded) | Phase 41 | §69 | `/ai/chat` amended |
| AI Content Studio (admin) | Phase 42 | §75 | `studio.*` RBAC |
| Participation queue (admin) | Phase 42 | §76 | `participation.review` RBAC |
| Visa Checklist | Phase 43 | §70 | `checklist.use` RBAC |
| Readiness Assessment | Phase 43 | §71 | `assessment.take` RBAC |
| Cost of Living Calculator | Phase 43 | §72 | `col_calculator.use` RBAC |
| Scholarship Finder | Phase 43 | §73 | `scholarship.browse` RBAC |
| Remittance Rate Tracker | Phase 44 | §74 | `remittance.view` RBAC |
| Referral system | Phase 45 | §60 | `referral.use` RBAC |
| Self-serve ads | Phase 45 | §61 | `ads.self_serve.create` RBAC |
| Newsletter | Phase 45 | §62 | `newsletter.subscribe` RBAC |
| Jobs board | Phase 45 | §63 | `jobs.browse` RBAC |
| Gamification | Phase 45 | §64 | `gamification.view` RBAC |
| Content import | Phase 46 | §65 | `import.use` RBAC |
| Advanced analytics | Phase 46 | §66 | `analytics.advanced` RBAC |
| Travel search | Phase 46 | §67 | `travel.search` RBAC |

**API-Readiness Gate applies to all v2 features.** Verify against `api-routes-catalog.md` — all v2 routes are marked as "V2 planned" in the catalog. Once the corresponding backend phase lands, the planned routes become available.

**New surface in v2:** `apps/tools` (§78, ADR-086) — a sixth SvelteKit SSR app at `tools.japaconnect.com` (port 3005). `jc-tools-*` skills are already generated and live at `.ai/skills/frontend/06-tools/`.
