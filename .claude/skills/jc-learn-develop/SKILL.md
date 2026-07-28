---
name: jc-learn-develop
description: Use when building new pages or components for the JapaConnect Learn app (apps/learn, port 3003, SSR, learn.japaconnect.com). Covers the unified app shell adapted for Learn (ADR-060), the lesson-player reading exception, the SSR/CSR progress boundary, TrackCard, LayerChart 2.0.1 progress analytics, and the API-Readiness Gate.
---

# JapaConnect Learn — Development Skill

Course/track platform at `learn.japaconnect.com`. SSR (adapter-node + Bun). API base `/api/v1/learn/*`. **Layout is the unified app shell (ADR-058) adapted for Learn (ADR-060) — build inside it; the lesson player is the one exception.**

## ⛔ API-READINESS GATE
Same as `jc-main-develop`: list endpoints → verify (catalog or probe) → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock. Known routes: `GET /learn/tracks`, `GET /learn/tracks/:slug`, `POST /learn/tracks/:id/enroll`, `GET /me/enrollments`, `GET /learn/lessons/:id`, `POST /learn/lessons/:id/progress`, `POST /learn/lessons/:id/complete`, `GET/POST /learn/quizzes/:id`, `GET /me/certificates` — verify each before use. **Japa University (ADR-063) routes**: `GET /learn/university/tracks` (marketplace catalog, filters on `is_published_to_university`), `POST /learn/university/instructors/apply` (creator application), `GET /me/instructor-profile` (own instructor status), `POST /learn/university/tracks/:id/submit` (submit course to marketplace), `GET /me/payouts` (instructor payout history) — verify each before use.

## Japa University Marketplace (ADR-063 — host-detected shell)

Japa University is the Learning Platform presented as a worldwide creator marketplace under its own brand. The Learn app **detects the host** (`university.japaconnect.com` default, or a custom domain from `system_settings.university.surface_domain`) and renders the Japa University shell. Not open sourced (ADR-063 §1).

- **Host detection:** in root `+layout.server.ts`, compare `request.headers.get('host')` against the configured `university.surface_domain`. If match → set `data.isUniversity = true` → render the Japa University shell variant (branded header, worldwide audience copy, creator-focused nav). If not → standard Learn shell.
- **Marketplace catalog:** `GET /learn/university/tracks` returns only `is_published_to_university = true` courses. `TrackCard` adds a price badge (`price_kobo` → formatted) and instructor name. Free and Pro-included courses show "Free" / "Pro".
- **Creator flow:** `POST /learn/university/instructors/apply` → `instructor_profiles.status = 'pending'`. Admin approves → `university:notify_instructor_approved`. Approved instructor creates a track → `POST /learn/university/tracks/:id/submit` → admin approves course → `university:notify_course_approved` → `is_published_to_university = true`.
- **Instructor dashboard:** `/dashboard/instructor` — course list with enrollment counts (`total_enrolled`), revenue overview from `GET /me/payouts`, pending/approved course status. LayerChart for revenue over time.
- **Pro dual-access:** Pro subscribers enroll in any university course for free. The UI shows "Included with Pro" instead of a price for Pro users. Creators are still compensated (Pro stream rate, ADR-063 §7) — this is backend, not visible in the UI.
- **Course reviews:** reuse the polymorphic `reviews` table (`entity_type = 'learning_track'`). Star ratings and counts from `review_aggregates`. No course-specific review UI — use the same review component as Section 29.
- **SSR/CSR boundary unchanged:** marketplace catalog is public (SSR); instructor dashboard and payout data are CSR (session-guarded).

## ⚠️ SSR/CSR BOUNDARY — Learn's Critical Rule
**User-specific data is NEVER server-rendered.** Progress, enrollment status, completed lessons, quiz scores → CSR only, guarded by session:
```svelte
// +page.server.ts — public content ONLY (track, curriculum, lesson HTML)
// +page.svelte — user data:
$effect(async () => {
  if (!data.session) return;
  const r = await fetch(`/api/v1/me/enrollments`);
  if (r.ok) enrollments = (await r.json()).data;
});
```
A `+page.server.ts` touching `/me/enrollments` or `/learn/lessons/:id/progress` is a critical defect (leaks one user's progress into cached SSR responses).

## The Shell (ADR-058 primitives, ADR-060 drift)
Adopt `AppShell` (collapsible `LeftNav` + center + `RightSidebar`), `ui.nav_collapsed` server-persistence (never localStorage), sticky right-sidebar search+avatar, dark-mode tokens, mobile single-column + bottom bar. Nav items from `GET /api/v1/menus/learn_nav` (ADR-054).
- **Left nav (learning-oriented):** Explore (catalog) · My Learning · Paths · Certificates · Bookmarks.
- **Center:** catalog grid / track detail / lesson player.
- **Right sidebar:** continue-learning card, streak + progress ring, up-next lesson.
- **`TrackCard`** is ONE variant-driven card (Ghost-style: thumbnail, title, `LevelBadge`, duration, instructor, rating, enrolled count, enroll/continue/locked). Never fork per level/type.

## Lesson Player — Learn's Reading Exception (ADR-060, dev.to-single analog)
Route `(app)/tracks/[slug]/[module]/[lesson]` (or `/learn/lessons/:id`). Left nav auto-collapses to 72px. Three zones:
- `CurriculumRail` — module→lesson tree in the content's LEFT margin: current (▶), completed (✓), locked (🔒), not-started (○). Sticky/scrollable. Belongs to the content — visually distinct from the collapsed nav, NOT a second nav.
- Center — lesson by `lesson_type`: `article` (SSR TipTap `.prose` + `.prose-table-wrapper`), `video` (browser-guarded init, `progress_secs` tracking), `quiz` (quiz engine), `checklist` (TaskList). Mark complete → `POST /learn/lessons/:id/complete` (optimistic `Set`, `marking` guard, rollback, auto-next).
- Right — lesson meta: track progress ring, Next lesson →, notes.
- No ads. Access verified server-side (403 if not enrolled and not `is_preview` → redirect to enroll).

## Track Detail (widened overview)
`/learn/tracks/[slug]`: SSR hero + stats · `<Tabs.Root>` Overview | Curriculum | Reviews · `<Accordion.Root type="multiple">` curriculum · instructor card · enroll CTA · `Course` JSON-LD.

## LayerChart — Learner Analytics (ADR-059)
Pinned `layerchart@2.0.1`. "My Learning" dashboard: progress over time (`LineChart`), weekly activity (`BarChart`), completion/time-spent, streak sparklines. High-level compound charts preferred:
```svelte
<script lang="ts">import { LineChart } from 'layerchart'; let { data } = $props();</script>
<div class="h-64 w-full"><LineChart {data} x="date" y="pct"
  props={{ spline: { class: 'stroke-[var(--chart-1)]' } }} tooltip /></div>
```
Rules: colors via `var(--chart-*)`/`var(--color-*)` only (dark-mode safe) · fixed-height containers · data from `/api/v1/me/*` + `/learn/*` (verify via Gate!) · per-chart skeleton · dates → `Date`.

## Difficulty & Progress Tokens (CSS custom properties, dark-mode safe — never hardcode hex)
`--level-beginner` (green) · `--level-intermediate` (amber) · `--level-advanced` (red) · `--color-progress` · `--color-locked`. Define once in the theme; reference as variables.

## Stack
Identical base to `jc-main-develop`: Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, LayerChart 2.0.1, Lucide, TypeScript, `$t()` i18n (en+fr), Rule 16, no AI attribution. Canonical URLs on `learn.japaconnect.com`.

## Deliverables Per Task
1. Gate output · 2. surface built INSIDE `AppShell` (or the lesson-player exception, explicitly) · 3. `+page.server.ts` (public content only — no user progress) · 4. `+page.svelte` (SSR content + CSR progress, skeletons) · 5. components · 6. JSON-LD · 7. types
