---
name: jc-main-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Main App (apps/web) before merge or release. Covers Rule 16, Svelte 5, Bits UI, LayerChart, SSR/SEO, security, accessibility, and i18n compliance.
---

# JapaConnect Main App — Audit Skill

Work file-by-file. Produce the structured report at the end. Conventions reference: `jc-main-develop`.

## A. API Consumption Verification (Run First)
For every `fetch()` and `load()` call found:
1. Extract the route · 2. Confirm it exists in JEOS `api-routes-catalog.md` or via live probe · 3. Any route the frontend calls that the backend doesn't serve → **Critical** finding with the ⚠️ MISSING BACKEND API ROUTES block listing them all.

## B. Architectural Compliance (per route file)
`load()` fetches from `/api/v1/*` (no static data) · no `any` types · `$props()`/`$derived()`/`$effect()` only · `onclick` not `on:click` · auth from `$page.data.session` · dark mode via `[data-theme]` · all strings `$t()`

## C. Rule 16
Flag every hardcoded array rendered to UI (nav, countries, tags, groups, post types, **chart datasets**) with file · line · violation · fix.

## D. Bits UI Compliance
All dropdowns/modals/selects/tooltips/accordions/tabs are Bits UI primitives · no manual `role`/`aria-*` on primitives · `<Dialog.Content>` inside `<Dialog.Portal>`.

## E. LayerChart Compliance
- Every chart on analytics/dashboard pages uses LayerChart — flag any Chart.js, raw D3 DOM, canvas, or hand-rolled SVG charts
- Chart colors exclusively `var(--color-*)` (verify dark-mode rendering)
- Fixed-height container wraps every `<Chart>`
- Chart data from API, dates converted to `Date` objects
- Skeleton state present per chart

## F. SSR & SEO
Indexable routes render content in server HTML · `<svelte:head>` with title/description on every indexable route · JSON-LD: Article (blog), QAPage (solved threads), Person/Organization (profiles) · images lazy · `data-sveltekit-preload-data="hover"` on body.

## G. Security
CSRF token on all mutations · `{@html}` only for TipTap-rendered backend content — flag all other instances as XSS risk · auth-required routes guarded server AND client side · no secrets in client code or `PUBLIC_JC_*`.

## H. Accessibility (WCAG 2.1 AA)
Meaningful `alt` · labeled buttons/links · logical focus order, no `tabindex>0` · visible focus rings · 4.5:1 contrast · no interactive `<div>` without role+keyboard handler.

## I. i18n
List hardcoded strings not in `$t()` · flag keys in `en.json` missing from `fr.json`.

## Report Format
```
## Audit Report — Main App (apps/web)
**Files Audited:** [n] · **Status:** PASS / NEEDS FIXES
### Critical (block merge) / High (this sprint) / Medium (before v1.0) / Low
[lists]
### Compliance Summary
| Category | Status |
| API routes verified | ✅/❌ |
| Rule 16 | ✅/❌ |
| Svelte 5 Runes | ✅/❌ |
| Bits UI | ✅/❌ |
| LayerChart | ✅/❌ |
| SSR/SEO | ✅/❌ |
| Security | ✅/❌ |
| Accessibility | ✅/❌ |
| i18n | ✅/❌ |
```
