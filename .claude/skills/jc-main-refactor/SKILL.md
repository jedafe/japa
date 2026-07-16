---
name: jc-main-refactor
description: Use when updating, modernizing, or refactoring existing JapaConnect Main App code (apps/web). Migrates Svelte 4 to Svelte 5 Runes, adopts Bits UI primitives, converts charts to LayerChart, removes hardcoded data per Rule 16.
---

# JapaConnect Main App — Updating & Refactoring Skill

Apply conventions from `jc-main-develop`. Do not change behavior — modernize implementation only. One-line comment per change.

## 0. API-Readiness Gate
If a refactor requires an endpoint that doesn't exist (e.g. replacing hardcoded data with an API call), verify the route first. Missing → output the ⚠️ alert and pause.

## 1. Svelte 4 → 5 Migration
`export let` → `$props()` · `$:` → `$derived()` · reactive side-effect blocks → `$effect()` · `createEventDispatcher` → callback props · writable stores for local UI state → `$state()` · `on:event` → `onevent`

## 2. Bits UI Adoption
| Manual pattern | Replace with |
|---|---|
| Custom dropdown `<ul>` toggle | `<DropdownMenu.Root>` |
| Custom modal + focus trap | `<Dialog.Root>` |
| Raw `<select>` | `<Select.Root>` / searchable `<Combobox.Root>` |
| CSS hover tooltip | `<Tooltip.Root>` |
| Manual accordion / tabs / popover | `<Accordion.Root>` / `<Tabs.Root>` / `<Popover.Root>` |

## 3. Chart Migration → LayerChart
Any chart built with Chart.js, D3 direct DOM, canvas, inline SVG math, or any other library **must** be rewritten in LayerChart:

```svelte
<!-- BEFORE: Chart.js canvas, manual lifecycle, breaks SSR -->
<canvas bind:this={el}></canvas>

<!-- AFTER: LayerChart, declarative, SSR-safe markup -->
<div class="h-64 w-full">
  <Chart {data} x="date" xScale={scaleTime()} y="count" yNice padding={{ left: 32, bottom: 24 }} tooltip>
    <Svg><Axis placement="left" grid /><Axis placement="bottom" /><Spline class="stroke-[var(--color-primary)] stroke-2" /></Svg>
  </Chart>
</div>
```
Convert API date strings to `Date` objects · colors via `var(--color-*)` only · add skeleton state · remove the old chart library from package.json if no longer used.

## 4. Rule 16 — Hardcoded Data Removal
Replace static arrays (nav, countries, tags, groups, post types, chart datasets) with API fetches. Verify the endpoint exists first (Gate 0).

## 5. Standard Patterns
- Skeletons: `{#if loading}<Skeleton/>{:else if error}<ErrorState/>{:else if empty}<EmptyState/>{:else}...{/if}` — skeleton shape matches real content
- Optimistic UI with rollback for reactions/bookmarks/follows
- Tailwind v4: remove `@apply`, `theme()`, `dark:` variant
- i18n sweep: hardcoded strings → `$t()`, keys in `en.json` + `fr.json`
- A11y: `<div onclick>` → `<button>`, `aria-label` on icon buttons, `alt` on images, visible focus rings
- Types: eliminate `any`, derive from API response types
