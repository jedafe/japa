---
name: jc-learn-debug
description: Use when reviewing or debugging JapaConnect Learn app code (apps/learn, SSR). Diagnoses SSR/CSR progress boundary leaks, TipTap rendering, video player browser APIs, Mark Complete races, and LayerChart issues.
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

## Output Protocol
1. Root cause · 2. Broken code · 3. Fixed code · 4. Secondary findings
