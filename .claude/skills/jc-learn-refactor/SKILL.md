---
name: jc-learn-refactor
description: [Learn app, learn.japaconnect.com] Unified shell adapted for Learn (ADR-060), lesson-player exception, LayerChart 2.0.1. Use when updating or refactoring existing JapaConnect Learn app code (apps/learn). Moves progress fetching out of SSR, modernizes lesson navigation to $derived, migrates charts to LayerChart.
---

# JapaConnect Learn — Updating & Refactoring Skill

Conventions and gates: `jc-learn-develop`. Behavior-preserving; one-line comment per change.

## 0. API-Readiness Gate
Refactor needs an endpoint → verify first (⚠️ alert if missing, pause).

## 1. Progress: SSR → CSR Migration (Priority 1)
Remove ALL progress/enrollment fetching from `+page.server.ts`; move to session-guarded `$effect` in the component (pattern in `jc-learn-develop`).

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

## 7. Japa University Marketplace (ADR-063)
- **Add host detection:** if root `+layout.server.ts` doesn't check the host against `university.surface_domain`, add it — set `data.isUniversity` and render the branded shell variant.
- **Split catalog endpoints:** if the marketplace catalog calls `/learn/tracks` (all tracks), switch to `/learn/university/tracks` (only `is_published_to_university = true`).
- **Add creator flow:** instructor application (`POST /learn/university/instructors/apply`), course submission (`POST /learn/university/tracks/:id/submit`), instructor dashboard (`/dashboard/instructor` with `GET /me/payouts`). Verify routes via Gate 0 first.
- **Price display:** raw `price_kobo` → formatted currency; NULL → "Free" or "Pro" based on user's Pro status.
- **Course reviews:** if a custom review component was built for courses, replace with the Section 29 polymorphic review component (`entity_type = 'learning_track'`).
- **Pro dual-access:** add Pro status check before rendering price — Pro users see "Included with Pro".
- **SSR/CSR boundary:** instructor dashboard and payout data must be CSR (session-guarded) — move any SSR payout/revenue fetch to `$effect`.

## App Shell & LayerChart (ADR-060 / ADR-059)
- Learn uses the unified shell (ADR-058) adapted per ADR-060: collapsible left nav (learning-oriented, state via `ui.nav_collapsed` server-side — not localStorage), center, right sidebar (continue/streak/up-next). Flag surfaces built as a top-nav or standalone layout.
- Lesson player is Learn's reading exception: collapsed nav + `CurriculumRail` (borderless, part of the content — not a second nav) + lesson-meta panel. Flag two-sidebar-looking rails.
- `TrackCard` is one variant-driven card — flag per-level/type card forks.
- Charts: pinned `layerchart@2.0.1`, colors via CSS vars, fixed height, Gate-verified data, per-chart skeleton — flag unpinned/`@next` or hardcoded chart colors.
- Enforce the SSR/CSR boundary: user progress/enrollment must be CSR, never in `+page.server.ts`.
