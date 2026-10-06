---
name: jc-academy-refactor
description: [Academy app, academy.japaconnect.com] Unified shell adapted for Academy (ADR-060), lesson-player exception, LayerChart 2.0.1. Use when updating or refactoring existing JapaConnect Academy app code (apps/academy). Moves progress fetching out of SSR, modernizes lesson navigation to $derived, migrates charts to LayerChart.
---

# JapaConnect Academy — Updating & Refactoring Skill

Conventions and gates: `jc-academy-develop`. Behavior-preserving; one-line comment per change.

## 0. API-Readiness Gate
Refactor needs an endpoint → verify first (⚠️ alert if missing, pause).

## 1. Progress: SSR → CSR Migration (Priority 1)
Remove ALL progress/enrollment fetching from `+page.server.ts`; move to session-guarded `$effect` in the component (pattern in `jc-academy-develop`).

## 2. Lesson Navigation → $derived Chain
```svelte
let allLessons = $derived(data.course.curriculum.flatMap(s => s.lessons));
let currentIndex = $derived(allLessons.findIndex(l => l.id === currentLessonId));
let prevLesson = $derived(allLessons[currentIndex - 1] ?? null);
let nextLesson = $derived(allLessons[currentIndex + 1] ?? null);
```

## 3. Completion: Array → Set
`number[]` + `includes()` → `$state(new Set<number>())` with `new Set()` reassignment; `progressPercent` as `$derived`.

## 4. Chart Migration → LayerChart
Any learner-activity or analytics chart rewritten in LayerChart (`jc-main-refactor` §3 pattern). Session-guard the data fetch.

## 5. Curriculum → Bits UI
Custom accordions → `<Accordion.Root type="multiple">`; custom tabs → `<Tabs.Root>`.

## 6. Standard Migrations
Apply `jc-main-refactor` §§1–2, 4–5 (Svelte 4→5, Bits UI, Rule 16, skeletons, Tailwind v4, i18n, a11y, types).

## 7. Japa Academy Marketplace (ADR-063)
- **Remove stale host detection:** the Academy shell is unified — delete any leftover `university.surface_domain` host checks or `isUniversity` locals; `academy.subdomain`/`academy.custom_domain` host routing lives at the edge.
- **Split catalog endpoints:** if the marketplace catalog calls `/academy/tracks` (all tracks), switch to `/academy/tracks` (only `is_published_to_academy = true`).
- **Add creator flow:** instructor application (`POST /academy/instructors/apply`), course submission (`POST /academy/tracks/:id/submit`), instructor dashboard (`/dashboard/instructor` with `GET /me/payouts`). Verify routes via Gate 0 first.
- **Price display:** raw `price_kobo` → formatted currency; NULL → "Free" or "Pro" based on user's Pro status.
- **Course reviews:** if a custom review component was built for courses, replace with the Section 29 polymorphic review component (`entity_type = 'academy_track'`).
- **Pro dual-access:** add Pro status check before rendering price — Pro users see "Included with Pro".
- **SSR/CSR boundary:** instructor dashboard and payout data must be CSR (session-guarded) — move any SSR payout/revenue fetch to `$effect`.

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

**New surface in v2:** `apps/tools` (§78, ADR-086) — a sixth SvelteKit SSR app at `tools.japaconnect.com` (port 3005). `jc-tools-*` skills are already generated and live at `.ai/skills/frontend/06-tools/`.
