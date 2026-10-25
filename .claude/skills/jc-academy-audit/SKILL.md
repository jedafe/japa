---
name: jc-academy-audit
description: [Academy app, academy.japa.to] Unified shell adapted for Academy (ADR-060), lesson-player exception, LayerChart 2.0.1. Use when performing a comprehensive quality audit of the JapaConnect Academy app (apps/academy) before merge or release. Verifies the SSR/CSR progress boundary, API route consumption, LayerChart, SEO structured data, and standard compliance.
---

# JapaConnect Academy — Audit Skill

Work file-by-file. Conventions: `jc-academy-develop`.

## A. SSR/CSR Boundary — Critical (Run First)
Scan every `+page.server.ts`:
- [ ] No progress, enrollment, completedIds, or quiz-score data in any SSR `load()`
- [ ] All user data fetched CSR with `if (!data.session) return` guard
- [ ] Course content (title, curriculum, lesson HTML) correctly SSR
**Flag every `+page.server.ts` touching `/api/v1/academy/progress/*` as Critical.**

## B. API Consumption Verification
Every `fetch()`/`load()` route confirmed against JEOS catalog or live probe. Uncatalogued → Critical + ⚠️ MISSING BACKEND API ROUTES block.

## C. Progress & Completion Logic
`Set<number>` for completedIds · `$derived` progressPercent · `marking` guard · optimistic + rollback · navigate-after-success only.

## D. TipTap & Media
`{@html}` in `.prose` only · imgproxy URLs · post-render highlighting (dynamic import, browser-guarded) · lazy iframes with titles.

## E. LayerChart Compliance
Learner dashboard / instructor analytics use LayerChart exclusively · `var(--color-*)` · fixed heights · skeletons · session-guarded data.

## F. SEO
Per route: `/courses` ItemList · `/courses/[slug]` Course JSON-LD · lesson pages LearningResource JSON-LD · all canonicals → `academy.japa.to` · titles + descriptions present.

## G. Performance
Lesson HTML SSR'd · highlight lib dynamically imported · lazy media · accordion renders only open sections.

## H. Standard Compliance
Apply `jc-main-audit` §§B (Rule 16), D (Bits UI), B(runes), H (a11y — plus progressbar roles, aria-disabled locked lessons, labeled quiz inputs), I (i18n).

## I. Japa Academy Marketplace (ADR-063)
- **Unified shell:** since the ADR-063 consolidation there is one Academy shell — no host-detection branching. The edge routes `academy.subdomain`/`academy.custom_domain` hosts to this app. Flag any leftover dual-shell host checks or client-side host detection.
- **Marketplace catalog:** `/academy/tracks` filters `is_published_to_academy = true` — flag if calling `/academy/tracks` instead (unpublished courses would leak).
- **Creator flow:** instructor application, course submission, and instructor dashboard routes all verified via Gate. Flag any hardcoded creator/marketplace data (Rule 16).
- **Price display:** `price_kobo` formatted as currency; NULL → "Free" or "Pro" (Pro status checked). Flag raw kobo display.
- **Pro dual-access:** Pro subscribers see "Included with Pro" — flag if price shown to Pro users.
- **Course reviews:** use polymorphic `reviews` (`entity_type = 'academy_track'`) — flag a custom/separate review system (ADR-063 §2 violation).
- **SSR/CSR boundary for marketplace:** instructor dashboard, payout data, and revenue are CSR (session-guarded) — flag any in SSR `load()` (same critical class as §A).
- **Instructor dashboard LayerChart:** revenue-over-time chart uses LayerChart, Gate-verified data, CSS-var colors, fixed height, skeleton.

## Report Format
Same structure as `jc-main-audit` with categories: SSR/CSR boundary · API routes · Rule 16 · Progress logic · LayerChart · Bits UI · Svelte 5 · SEO · Performance · A11y · **Japa Academy marketplace (ADR-063)**.

## App Shell & LayerChart (ADR-060 / ADR-059)
- Academy uses the unified shell (ADR-058) adapted per ADR-060: collapsible left nav (learning-oriented, state via `ui.nav_collapsed` server-side — not localStorage), center, right sidebar (continue/streak/up-next). Flag surfaces built as a top-nav or standalone layout.
- Lesson player is Academy's reading exception: collapsed nav + `CurriculumRail` (borderless, part of the content — not a second nav) + lesson-meta panel. Flag two-sidebar-looking rails.
- `TrackCard` is one variant-driven card — flag per-level/type card forks.
- Charts: pinned `layerchart@2.0.1`, colors via CSS vars, fixed height, Gate-verified data, per-chart skeleton — flag unpinned/`@next` or hardcoded chart colors.
- Enforce the SSR/CSR boundary: user progress/enrollment must be CSR, never in `+page.server.ts`.
**V2 academy app:** No new pages in v2. Japa Academy (ADR-063) is already implemented. The academy app may receive minor amendments if Japa Academy course visibility is affected by v2 persona authorship or the AI Content Studio publishing courses. **Per ADR-060, v2 community features (§§79–89), functional tools (§§90–101), and mentorship (§102) are not part of the academy platform** — they live in the Main app (community) and Tools app (functional tools). However, academy content may be cross-referenced from these features (e.g. a functional tool may link to a relevant academy track). Total platform route count is now ~819 (~475 v1 + ~344 v2); 18 Golden Rules (R18: always use bun).

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

**Removed surface:** `apps/tools` was consolidated into `apps/main` (`/tools/{tool}`) — ADR-086 superseded. `tools.japa.to` and per-tool subdomains 301 to `japa.to/tools/{tool}`; the `jc-tools-*` skills were deleted.
