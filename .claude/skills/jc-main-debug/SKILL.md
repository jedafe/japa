---
name: jc-main-debug
description: Use when reviewing or debugging code in the JapaConnect Main App (apps/web, SSR). Diagnoses SSR/hydration mismatches, Svelte 5 Runes violations, Bits UI lifecycle errors, LayerChart rendering issues, and Rule 16 violations.
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
- Hardcoded hex colors in chart classes → breaks dark mode; must use `var(--color-*)`
- Chart data hardcoded → Rule 16 violation
- `scaleTime`/`scaleLinear` not imported from `d3-scale`
- Date strings from API not converted to `Date` objects before passing to `<Chart x="date" xScale={scaleTime()}>`
- Missing skeleton state while chart data loads
- `<Tooltip.Root>` (LayerChart's) confused with Bits UI `<Tooltip.Root>` — check import source

## 5. Rule 16 Violations
- Hardcoded arrays for nav, tags, spaces, countries, post types, chart data
- `load()` returning static data · mutations updating only local `$state`
- Optimistic updates without rollback on API failure

## 6. Tailwind v4
- `@apply` in `<style>` → inline utilities · `theme()` → `var(--color-*)` · `dark:` variant → `[data-theme="dark"]`

## 7. SEO & A11y
- Indexable routes need `<svelte:head>` title/description/OG + JSON-LD (Article/QAPage/Person/Organization)
- Images: `loading="lazy" decoding="async"` · icon buttons: `aria-label` · `<div onclick>` → `<button>` · focus rings visible

## Output Protocol
1. **Root cause** — one sentence · 2. **Broken code** · 3. **Fixed code** (surgical, minimal) · 4. **Secondary findings**
