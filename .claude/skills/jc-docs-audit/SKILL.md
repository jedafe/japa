---
name: jc-docs-audit
description: [Docs app, docs.japaconnect.com] Unified shell adapted for Docs (ADR-060) — nav-tree left nav, TOC right sidebar, playground full-width exception. Use when performing a comprehensive quality audit of the JapaConnect Docs app (apps/docs) before merge or release. Verifies observer/listener cleanup, API-driven sidebar, search modal composition, code blocks, SEO structured data, and standard compliance.
---

# JapaConnect Docs — Audit Skill

Work file-by-file. Conventions: `jc-docs-develop`.

## A. Memory Leak Audit — Critical (Run First)
| Subscription | Required cleanup |
|---|---|
| IntersectionObserver | `$effect` returns `disconnect()` |
| window keydown (Cmd+K) | `$effect` returns `removeEventListener` |
| setTimeout debounce | `$effect` returns `clearTimeout` |
| setInterval / ResizeObserver | cleared/disconnected |
Verify by navigating between 10 docs pages — single observer/listener set in DevTools. Any missing cleanup → Critical.

## B. API Consumption Verification
Every route confirmed against catalog/probe (⚠️ block if missing). Sidebar MUST come from `/api/v1/docs/sections` — hardcoded tree = Critical Rule 16 violation.

## C. Search Modal
`<Dialog.Root>` + `<Combobox.Root>` composition · autofocus · 200ms debounce · grouped results · full keyboard path (Cmd+K → arrows → Enter → Escape).

## D. Code Blocks
Language badge · copy button (textContent, tooltip confirmation, screen-reader announce) · post-render dynamic-import highlighting · dark-mode contrast of syntax colors.

## E. On-This-Page
Live-DOM heading extraction · IntersectionObserver tracking · smooth anchor scroll · hidden on mobile · `role="navigation"` + aria-label.

## F. Responsive Layout
Column collapse at breakpoints · `<Dialog.Root>` mobile drawer with focus trap · no horizontal overflow.

## G. SEO & Structured Data
Per page: title, description, canonical → `docs.japaconnect.com`, TechArticle JSON-LD · landing: WebSite + SearchAction JSON-LD · API reference: appropriate WebPage schema.

## H. LayerChart (If Usage Stats Present)
Apply `jc-main-audit` §E.

## I. Standard Compliance
Apply `jc-main-audit` §§B (Rule 16), D (Bits UI), B(runes), G (security/{@html}), H (a11y — plus code-region roles, copy-button labels, nav landmarks, external-link rel), I (i18n).

## J. Japa Academy & Geo-Aware API Docs (ADR-063/064)
- API reference pages exist for the new v1.11.0 routes: `GET /geo/me`, `GET /geo/lookup`, `/academy/*`, `/me/instructor-profile`, `/me/payouts`, `/universities`, `/admin/universities` — flag any missing
- `?geo=` param behavior documented on feed/search/trending/discovery routes (defaults to `ContentCountry`, explicit `?geo=all` for global)
- `price_kobo` documented as integer (kobo), `is_published_to_academy` documented as distinct from `is_published`
- University spaces (Section 39) `university_fields` custom field group documented
- All new routes use the three-column API-docs variant with method badges and Request/Response/Example tabs

## Report Format
Same structure as `jc-main-audit` with categories: Memory leaks · API routes/sidebar · Search modal · Code blocks · On-This-Page · Responsive · SEO · Bits UI · Svelte 5 · A11y · **Japa Academy/Geo API docs (ADR-063/064)**.

## App Shell & Docs Drift (ADR-060)
- Docs uses the unified shell (ADR-058) adapted per ADR-060: the **left nav IS the audience-gated doc nav tree** (from `GET /api/v1/docs/nav`), collapsible (`ui.nav_collapsed` server-side, not localStorage); the **right sidebar IS the in-page TOC** + "was this helpful?" — flag social-widget sidebars or a hardcoded nav tree.
- API reference adds a right code-sample pane; the API Playground is the full-width tool exception — flag a playground crammed into the three-column shell.
- Audience gating is server-side only (403 → `/docs/access-denied`) — flag any client-side permission hiding.
- ⌘K search + TOC observers must clean up in `$effect` return.
- Any usage-stat charts use pinned `layerchart@2.0.1`, CSS-var colors, fixed height.
**V2 docs app:** No new pages in v2. Docs app requires stable API surface (OpenAPI spec from §47). The `japaconnect_core` Flutter package will use the same OpenAPI spec — ensure spec remains current when v2 backend phases land. **JEOS v1.22.0–v1.23.1 updates (§§79–102, ADRs 087–110):**
- **OpenAPI spec must cover the new ~344 v2 routes** (§§60–102) — flag if the spec from §47 doesn't include v2 community feature routes (§§79–89, §98), functional tool routes (§§90–97, §§99–101), and mentorship routes (§102). Total platform route count is now ~819 (~475 v1 + ~344 v2).
- **New ADRs 087–110 need documentation pages** — flag any missing ADR docs pages. Key ADRs: ADR-088 (universal AI content generation), ADR-042 (feature-flag 404 not 403), ADR-110 (mentorship matching).
- **New sections §§79–102 need API reference documentation** — 24 new sections covering 11 v2 community features, 12 V2 functional tools, and mentorship matching. Flag any missing API reference pages.
- **Geo-aware tool pattern needs a docs page** — `.jeos/skills/geo-aware-tool-pattern.md` needs a public docs page explaining the `?dest=` filter, `destination_country_id` schema field, destination selector UX, and feature-flag 404 behavior. Flag if missing.
- **ADR-088 AI content generation columns** need documentation: `ai_generated`, `ai_model`, `hitl_reviewed_by`, `ai_generated_at`, `is_japabot_indexed` — flag if these standard columns are not documented for v2 tool content tables.
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
