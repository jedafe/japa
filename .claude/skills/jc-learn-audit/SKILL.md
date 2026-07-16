---
name: jc-learn-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Learn app (apps/learn) before merge or release. Verifies the SSR/CSR progress boundary, API route consumption, LayerChart, SEO structured data, and standard compliance.
---

# JapaConnect Learn — Audit Skill

Work file-by-file. Conventions: `jc-learn-develop`.

## A. SSR/CSR Boundary — Critical (Run First)
Scan every `+page.server.ts`:
- [ ] No progress, enrollment, completedIds, or quiz-score data in any SSR `load()`
- [ ] All user data fetched CSR with `if (!data.session) return` guard
- [ ] Course content (title, curriculum, lesson HTML) correctly SSR
**Flag every `+page.server.ts` touching `/api/v1/learn/progress/*` as Critical.**

## B. API Consumption Verification
Every `fetch()`/`load()` route confirmed against JEOS catalog or live probe. Uncatalogued → Critical + ⚠️ MISSING BACKEND API ROUTES block.

## C. Progress & Completion Logic
`Set<number>` for completedIds · `$derived` progressPercent · `marking` guard · optimistic + rollback · navigate-after-success only.

## D. TipTap & Media
`{@html}` in `.prose` only · imgproxy URLs · post-render highlighting (dynamic import, browser-guarded) · lazy iframes with titles.

## E. LayerChart Compliance
Learner dashboard / instructor analytics use LayerChart exclusively · `var(--color-*)` · fixed heights · skeletons · session-guarded data.

## F. SEO
Per route: `/courses` ItemList · `/courses/[slug]` Course JSON-LD · lesson pages LearningResource JSON-LD · all canonicals → `learn.japaconnect.com` · titles + descriptions present.

## G. Performance
Lesson HTML SSR'd · highlight lib dynamically imported · lazy media · accordion renders only open sections.

## H. Standard Compliance
Apply `jc-main-audit` §§B (Rule 16), D (Bits UI), B(runes), H (a11y — plus progressbar roles, aria-disabled locked lessons, labeled quiz inputs), I (i18n).

## Report Format
Same structure as `jc-main-audit` with categories: SSR/CSR boundary · API routes · Rule 16 · Progress logic · LayerChart · Bits UI · Svelte 5 · SEO · Performance · A11y.
