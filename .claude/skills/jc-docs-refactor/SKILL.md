---
name: jc-docs-refactor
description: [Docs app, docs.japaconnect.com] Unified shell adapted for Docs (ADR-060) — nav-tree left nav, TOC right sidebar, playground full-width exception. Use when updating or refactoring existing JapaConnect Docs app code (apps/docs). Converts scroll tracking to IntersectionObserver, modernizes search to Dialog+Combobox, fixes listener cleanup, migrates any charts to LayerChart.
---

# JapaConnect Docs — Updating & Refactoring Skill

Conventions and gates: `jc-docs-develop`. Behavior-preserving; one-line comment per change.

## 0. API-Readiness Gate
Refactor needs an endpoint → verify first (⚠️ alert if missing, pause). Hardcoded sidebar tree found → replace with `/api/v1/docs/sections` fetch (verify route first).

## 1. Scroll Tracking → IntersectionObserver
Replace scroll-event heading tracking with IntersectionObserver in `$effect` + `disconnect()` cleanup (`rootMargin: '-10% 0% -80% 0%'`).

## 2. Global Listeners → $effect with Cleanup
onMount/onDestroy listener pairs (Cmd+K, resize) → single `$effect` returning the remove call.

## 3. Search Debounce Modernization
`$:` + manual clearTimeout → `$effect` with `return () => clearTimeout(id)`; fetch `/docs/search` on settled query.

## 4. Search Modal → Dialog + Combobox
Custom overlays → `<Dialog.Root bind:open>` + `<Combobox.Root>` composition (grouped results, keyboard nav).

## 5. Mobile Sidebar → Dialog.Root
Custom drawers → `<Dialog.Root>` for focus trapping.

## 6. Chart Migration → LayerChart
Any usage-stat/analytics chart rewritten in LayerChart (`jc-main-refactor` §3 pattern).

## 7. Standard Migrations
Apply `jc-main-refactor` §§1–2, 4–5 (Svelte 4→5, Bits UI — incl. `<Tabs.Root>` for API reference, Rule 16, skeletons, Tailwind v4, i18n, a11y, types).

## 8. Japa University & Geo API Docs (ADR-063/064)
- **Add API reference pages** for new v1.11.0 routes if missing: `/geo/me`, `/geo/lookup`, `/learn/university/*`, `/me/instructor-profile`, `/me/payouts`, `/universities`, `/admin/universities`. Use the three-column API-docs variant with method badges and Request/Response/Example tabs.
- **Document `?geo=` param** on feed/search/trending/discovery route docs — defaults to `ContentCountry` when absent, `?geo=all` for global.
- **Document `price_kobo`** as integer in kobo; document `is_published_to_university` as distinct from `is_published`.
- **Document `university_fields`** custom field group for Section 39 university spaces.

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
