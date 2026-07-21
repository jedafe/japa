---
name: jc-main-debug
description: Use when reviewing or debugging code in the JapaConnect Main App (apps/web, SSR). Diagnoses SSR/hydration mismatches, Svelte 5 Runes violations, Bits UI lifecycle errors, app-shell/layout drift (ADR-058), profile learning hub and cross-app link defects (ADR-060), LayerChart rendering issues, and Rule 16 violations.Use when reviewing or debugging code in the JapaConnect Main App (apps/web, SSR). Diagnoses SSR/hydration mismatches, Svelte 5 Runes violations, Bits UI lifecycle errors, app-shell/layout drift (ADR-058), LayerChart rendering issues, and Rule 16 violations.
---

# JapaConnect Main App — Review & Debugging Skill

Apply the stack conventions from `jc-main-develop`. Run every check in order.

## 0. API-Readiness Check First
If the bug involves data not loading: verify the endpoint actually exists (`curl` the route) **before** debugging frontend code. If the route is missing, output the ⚠️ MISSING BACKEND API ROUTES alert (format in `jc-main-develop`) and pause.

## 1. SSR / Hydration
- `window`/`document`/`navigator` access without `if (browser)` guard from `$app/environment`
- Bits UI primitives instantiated server-side without guard
- `load()` data diverging between server and client render → hydration mismatch
- `jc_theme` cookie read in root layout **before first paint** (else theme flash)
- Auth from `$page.data.session` — never localStorage

## 2. Svelte 5 Runes Violations
| Forbidden | Correct |
|---|---|
| `export let foo` | `let { foo } = $props()` |
| `$: d = x * 2` | `let d = $derived(x * 2)` |
| `$: { sideEffect() }` | `$effect(() => { ... })` |
| `on:click` | `onclick` |
| `createEventDispatcher()` | callback props |

## 3. Bits UI Errors
- v1 single-component API (`<Select>`) instead of dot-notation (`<Select.Root>`)
- `e.stopPropagation()` on `<DropdownMenu.Trigger>` — breaks dismiss logic
- `<Dialog.Content>` outside `<Dialog.Portal>` — breaks stacking/focus trap
- Manual `role`/`aria-*` on primitives — remove, Bits UI manages ARIA
- `bind:value` on Trigger instead of Root · plain `<Select>` where searchable `<Combobox.Root>` is needed

## 4. LayerChart Issues
- Chart rendered without fixed-height container → collapses to 0px
- Hardcoded hex colors in chart classes → breaks dark mode; must use `var(--chart-*)`/`var(--color-*)`
- Chart data hardcoded → Rule 16 violation
- Wrong version: must be pinned `layerchart@2.0.1` (Svelte 5). Low-level `<Chart>` needs scales from `d3-scale`; high-level `AreaChart`/`LineChart`/`BarChart` do not (d3-scale is bundled)
- Date strings from API not converted to `Date` objects before passing to a time axis
- Missing skeleton state while chart data loads
- `<Tooltip.Root>` (LayerChart's) confused with Bits UI `<Tooltip.Root>` — check import source

## 4b. App Shell & Layout Drift (ADR-058)
- A surface built as a standalone layout or with a top-nav bar instead of rendering inside `AppShell` (three-column shell) — flag unless it is one of the three exceptions (blog/CPT single, `/create`, guest homepage)
- `FeedCard` forked per content type (separate components / large `{#if content_type}` layout branches) instead of one variant-driven component with a type pill
- Left-nav collapse state read/written to `localStorage` instead of `ui.nav_collapsed` via `PATCH /api/v1/me/account`
- Account dropdown placed in the left nav (should anchor to the right-sidebar avatar); Space switcher moved out of the left nav
- Blog/CPT single: `ReactionRail` boxed/bordered so it looks like a second nav (must be transparent, part of the article); search/avatar bar shown instead of `CreatorCard`
- `/create`: right panel is a plain settings drawer instead of the tabbed `[ Guide | Settings/SEO ]`; missing dynamic field-focus Guide
- Guest `/` rendering the app shell instead of the Page Builder landing (or authenticated `/` not showing the Primary timeline)

## 5. Rule 16 Violations
- Hardcoded arrays for nav, tags, spaces, countries, post types, chart data
- `load()` returning static data · mutations updating only local `$state`
- Optimistic updates without rollback on API failure

## 6. Tailwind v4
- `@apply` in `<style>` → inline utilities · `theme()` → `var(--color-*)` · `dark:` variant → `[data-theme="dark"]`

## 7. SEO & A11y
- Indexable routes need `<svelte:head>` title/description/OG + JSON-LD (Article/QAPage/Person/Organization)
- Images: `loading="lazy" decoding="async"` · icon buttons: `aria-label` · `<div onclick>` → `<button>` · focus rings visible

## 7b. Profile Learning Hub & Cross-App (ADR-060)
- Certificates, enrollments, or knowledge-base lists hardcoded instead of fetched from `GET /api/v1/me/certificates` / `GET /api/v1/me/enrollments` (Rule 16 violation)
- Learn or Docs surfaces **re-implemented inside the main app** (a lesson player, curriculum rail, or docs nav tree built here) — those belong to the `jc-learn-*` / `jc-docs-*` apps; the main app links out by absolute subdomain URL
- Cross-app links hardcoded to a wrong/relative path instead of `learn.japaconnect.com` / `docs.japaconnect.com` (breaks the shared-session deep link)
- Certificate cards not linking to the public shareable page `/certificates/:id`; earned certificates missing as badges on the public profile
- Dashboard treated as a public surface — it is private: no ad slots, and it must not be indexed

## Output Protocol
1. **Root cause** — one sentence · 2. **Broken code** · 3. **Fixed code** (surgical, minimal) · 4. **Secondary findings**
