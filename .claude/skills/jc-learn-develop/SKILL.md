---
name: jc-learn-develop
description: Use when building new pages or components for the JapaConnect Learn app (apps/learn, port 3003, SSR, learn.japaconnect.com). Covers the SSR/CSR progress boundary, course/lesson layouts, LayerChart progress analytics, and the API-Readiness Gate.
---

# JapaConnect Learn — Development Skill

Course/lesson platform at `learn.japaconnect.com`. SSR (adapter-node + Bun). API base `/api/v1/learn/*`. Design: dev.to reading experience + Coursera course layout.

## ⛔ API-READINESS GATE
Same as `jc-main-develop`: list endpoints → verify (catalog or probe) → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock. Known routes: courses list/detail, lessons/:id, progress/:course_id, enroll, progress PATCH, reviews GET/POST, quizzes submit — verify each before use.

## ⚠️ SSR/CSR BOUNDARY — Learn's Critical Rule
**User-specific data is NEVER server-rendered.** Progress, enrollment status, completed lessons, quiz scores → CSR only, guarded by session:
```svelte
// +page.server.ts — public content ONLY (course, curriculum, lesson HTML)
// +page.svelte — user data:
$effect(async () => {
  if (!data.session) return;
  const r = await fetch(`/api/v1/learn/progress/${data.course.id}`);
  if (r.ok) progress = (await r.json()).data;
});
```
A `+page.server.ts` touching `/api/v1/learn/progress/*` is a critical defect (leaks one user's progress into cached SSR responses).

## LayerChart — Learner Analytics
Learner dashboard ("My Learning") and instructor/course analytics use LayerChart:
- Weekly learning activity: `<Bars>` (lessons completed per day)
- Course progress over time: `<Area>` + `<Spline>`
- Quiz score distribution: `<Bars>`
- Streak/stat cards with sparklines
Rules: `var(--color-*)` colors · fixed-height containers · data from `/api/v1/learn/*` analytics endpoints (verify via Gate!) · skeletons · dates → `Date` objects.

## Page Layouts
**Course listing `/courses`:** card grid (cover, title, instructor, duration, lesson count, difficulty badge, rating stars, Enroll/Continue) · filter (difficulty/duration/free-paid) · sort · Meilisearch debounced search `?q=`.
**Course overview `/courses/[slug]`:** SSR hero + stats · sticky sidebar (desktop) / bottom CTA bar (mobile) · `<Tabs.Root>` Overview | Curriculum | Reviews · `<Accordion.Root type="multiple">` curriculum · `Course` JSON-LD.
**Lesson `/courses/[slug]/[lesson]`:** SSR TipTap HTML in `.prose` · CSR progress · two-column desktop (sticky lesson-list sidebar) / drawer mobile · video top (browser-guarded init) · Mark Complete (optimistic Set<number>, `marking` guard, rollback, auto-next on success) · prev/next nav · `LearningResource` JSON-LD.

## Difficulty & Progress Tokens
Beginner `#10B981` · Intermediate `#F59E0B` · Advanced `#EF4444` · `--color-progress #28A745` · `--color-locked #9CA3AF` — as CSS custom properties.

## Stack
Identical base to `jc-main-develop`: Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, LayerChart, Lucide, TypeScript, `$t()` i18n (en+fr), Rule 16, no AI attribution. Canonical URLs on `learn.japaconnect.com`.

## Deliverables Per Task
1. Gate output · 2. `+page.server.ts` (public content only) · 3. `+page.svelte` (SSR content + CSR progress, skeletons) · 4. components · 5. JSON-LD · 6. types
