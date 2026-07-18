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

## App Shell & LayerChart (ADR-060 / ADR-059)
- Learn uses the unified shell (ADR-058) adapted per ADR-060: collapsible left nav (learning-oriented, state via `ui.nav_collapsed` server-side — not localStorage), center, right sidebar (continue/streak/up-next). Flag surfaces built as a top-nav or standalone layout.
- Lesson player is Learn's reading exception: collapsed nav + `CurriculumRail` (borderless, part of the content — not a second nav) + lesson-meta panel. Flag two-sidebar-looking rails.
- `TrackCard` is one variant-driven card — flag per-level/type card forks.
- Charts: pinned `layerchart@2.0.1`, colors via CSS vars, fixed height, Gate-verified data, per-chart skeleton — flag unpinned/`@next` or hardcoded chart colors.
- Enforce the SSR/CSR boundary: user progress/enrollment must be CSR, never in `+page.server.ts`.
