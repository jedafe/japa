---
name: jc-learn-debug
description: [Learn app, learn.japaconnect.com] Unified shell adapted for Learn (ADR-060), lesson-player exception, LayerChart 2.0.1. Use when reviewing or debugging JapaConnect Learn app code (apps/learn, SSR). Diagnoses SSR/CSR progress boundary leaks, TipTap rendering, video player browser APIs, Mark Complete races, and LayerChart issues.
---

# JapaConnect Learn — Review & Debugging Skill

Conventions and gates: `jc-learn-develop`. Run in order.

## 0. API-Readiness First
Data not loading → verify the route exists before debugging frontend. Missing → ⚠️ MISSING BACKEND API ROUTES alert, pause.

## 1. SSR/CSR Boundary (Learn's #1 Bug — Critical)
- ANY `+page.server.ts` fetching `/api/v1/learn/progress/*`, enrollment status, or completed lessons → Critical defect (progress leaks between users via cached SSR)
- CSR progress fetch guarded by `if (!data.session) return`
- SSR HTML contains no user-specific data (names, progress %, emails)
- Course content (title, curriculum structure, lesson HTML) correctly SSR

## 2. TipTap Content
`{@html}` inside `.prose` · images via imgproxy URLs · syntax highlighting post-render (`$effect` + `if (!browser) return` + dynamic import) — never in SSR bundle.

## 3. Video Player
Init browser-guarded · destroyed on unmount · iframe embeds have `title` attribute + `loading="lazy"`.

## 4. Mark Complete Races
`marking` guard prevents concurrent PATCH · `completedIds` is `Set<number>` with `new Set()` reassignment for reactivity · optimistic + rollback · next-lesson navigation only after successful PATCH.

## 5. Curriculum & Progress
`<Accordion.Root type="multiple">` · locked lessons: lock icon, non-navigable, `aria-disabled` · active lesson highlighted · `progressPercent = $derived(completedIds.size / totalLessons * 100)` reactive to completion.

## 6. LayerChart
Apply `jc-main-debug` §4 (containers, colors, Date conversion, skeletons, tooltip import alias). Learn-specific: activity charts must not render before session-guarded data loads.

## 7. SEO
`Course` JSON-LD on overview, `LearningResource` on lessons · canonicals point to `learn.japaconnect.com`.

## 8. Standard Checks
Apply `jc-main-debug` §§1–3, 5–7 (SSR/hydration, Runes, Bits UI, Tailwind, a11y).

## 9. Japa University Marketplace (ADR-063)
- **Host detection broken:** university shell not rendering on `university.japaconnect.com` — check root `+layout.server.ts` host comparison against `system_settings.university.surface_domain`. If the setting is empty, it defaults to `university.japaconnect.com`.
- **Marketplace catalog shows unpublished courses:** `GET /learn/university/tracks` must filter `is_published_to_university = true` — if draft/unapproved courses appear, the backend filter is missing or the frontend is calling the wrong endpoint (`/learn/tracks` instead of `/learn/university/tracks`).
- **Creator application status stuck:** `instructor_profiles.status` not updating after admin approval — check the admin approval route and `university:notify_instructor_approved` task wiring.
- **Price display wrong:** `price_kobo` is in kobo (integer) — display as formatted currency. NULL price = "Free" or "Pro" (check user's Pro status). Don't show raw kobo value.
- **Instructor dashboard data leak:** payout/revenue data must be CSR (session-guarded) — if it appears in SSR HTML, that's a critical SSR/CSR boundary violation (same class as §1).
- **Pro dual-access not reflected:** Pro subscriber sees a price instead of "Included with Pro" — check Pro status check before rendering price.
- **Course reviews missing:** reviews should use the polymorphic `reviews` table (`entity_type = 'learning_track'`) — if a separate review system was built, that's a defect (ADR-063 §2: reuse, no `course_reviews` table).

## Output Protocol
1. Root cause · 2. Broken code · 3. Fixed code · 4. Secondary findings

## App Shell & LayerChart (ADR-060 / ADR-059)
- Learn uses the unified shell (ADR-058) adapted per ADR-060: collapsible left nav (learning-oriented, state via `ui.nav_collapsed` server-side — not localStorage), center, right sidebar (continue/streak/up-next). Flag surfaces built as a top-nav or standalone layout.
- Lesson player is Learn's reading exception: collapsed nav + `CurriculumRail` (borderless, part of the content — not a second nav) + lesson-meta panel. Flag two-sidebar-looking rails.
- `TrackCard` is one variant-driven card — flag per-level/type card forks.
- Charts: pinned `layerchart@2.0.1`, colors via CSS vars, fixed height, Gate-verified data, per-chart skeleton — flag unpinned/`@next` or hardcoded chart colors.
- Enforce the SSR/CSR boundary: user progress/enrollment must be CSR, never in `+page.server.ts`.
**V2 learn app:** No new pages in v2. Japa University (ADR-063) is already implemented. The learn app may receive minor amendments if Japa University course visibility is affected by v2 persona authorship or the AI Content Studio publishing courses.

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

**New surface in v2:** `apps/tools` (§78, ADR-086) — a sixth SvelteKit SSR app at `tools.japaconnect.com` (port 3005). Generate `jc-tools-*` skills before implementing this surface. Skills live at `.ai/skills/frontend/06-tools/`.
