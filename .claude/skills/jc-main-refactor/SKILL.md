---
name: jc-main-refactor
description: Use when updating, modernizing, or refactoring existing JapaConnect Main App code (apps/web). Migrates Svelte 4 to Svelte 5 Runes, adopts Bits UI primitives, migrates legacy layouts to the unified app shell (ADR-058), consolidates the profile learning hub and cross-app links (ADR-060), converts charts to LayerChart 2.0.1, removes hardcoded data per Rule 16.Use when updating, modernizing, or refactoring existing JapaConnect Main App code (apps/web). Migrates Svelte 4 to Svelte 5 Runes, adopts Bits UI primitives, migrates legacy layouts to the unified app shell (ADR-058), converts charts to LayerChart 2.0.1, removes hardcoded data per Rule 16.
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

## 3. Chart Migration → LayerChart 2.0.1
Any chart built with Chart.js, D3 direct DOM, canvas, inline SVG math, or any other library **must** be rewritten in LayerChart (pinned `layerchart@2.0.1`, prefer high-level compound charts):

```svelte
<!-- BEFORE: Chart.js canvas, manual lifecycle, breaks SSR -->
<canvas bind:this={el}></canvas>

<!-- AFTER: LayerChart v2 compound chart, declarative, SSR-safe -->
<div class="h-64 w-full">
  <AreaChart {data} x="date" y="count"
    props={{ area: { class: 'fill-[var(--chart-1)]/20 stroke-[var(--chart-1)]' } }} tooltip />
</div>
```
Use `AreaChart`/`LineChart`/`BarChart` for the mapped dashboard widgets (ADR-059); drop to the low-level `<Chart><Svg>…</Svg></Chart>` only when a compound chart can't express it. Convert API date strings to `Date` objects · colors via `var(--chart-*)`/`var(--color-*)` only · add skeleton state · pin `layerchart@2.0.1` and remove the old chart library from package.json if unused.

## 3b. Legacy Layout → Unified App Shell (ADR-058)
Migrate pre-shell layouts to the single three-column shell:
- Replace any top-nav-bar layout / bespoke per-page layout with `AppShell` (`LeftNav` + center + `RightSidebar`) — except the three exceptions (blog/CPT single, `/create`, guest homepage).
- Collapse a per-content-type `FeedCard` (separate components or big `{#if content_type}` layout branches) into one variant-driven `FeedCard` with a content-type pill + additive props.
- Move nav-collapse state from `localStorage` to `ui.nav_collapsed` (`PATCH /api/v1/me/account`); move the account dropdown to the right-sidebar avatar; keep the Space switcher in the left nav.
- Blog/CPT single: make the reaction rail borderless/transparent (`ReactionRail`); replace the search/avatar bar with `CreatorCard`.
- `/create`: convert the settings drawer into the tabbed `[ Guide | Settings/SEO ]` panel with the dynamic field-focus Guide.
- Guest `/`: render the Page Builder landing (fallback `/explore`); keep authenticated `/` on the Primary timeline.

## 4. Rule 16 — Hardcoded Data Removal
Replace static arrays (nav, countries, tags, groups, post types, chart datasets) with API fetches. Verify the endpoint exists first (Gate 0).

## 5b. Profile Learning Hub & Cross-App (ADR-060)
- Replace hardcoded certificate/enrollment/knowledge-base lists with `GET /api/v1/me/certificates`, `GET /api/v1/me/enrollments`, and the bookmark routes (no new backend needed).
- Extract any Learn/Docs surface re-implemented in this app (lesson player, curriculum rail, docs nav tree) — replace with an absolute deep link to `learn.japaconnect.com` / `docs.japaconnect.com`; those apps own those surfaces.
- Consolidate scattered learning links into the dashboard hub tabs: `/dashboard/learning`, `/dashboard/certificates`, `/dashboard/knowledge-base`.
- Point certificate cards at the public shareable page `/certificates/:id`; add earned-certificate badges to the public profile.

## 6. Standard Patterns
- Skeletons: `{#if loading}<Skeleton/>{:else if error}<ErrorState/>{:else if empty}<EmptyState/>{:else}...{/if}` — skeleton shape matches real content
- Optimistic UI with rollback for reactions/bookmarks/follows
- Tailwind v4: remove `@apply`, `theme()`, `dark:` variant
- i18n sweep: hardcoded strings → `$t()`, keys in `en.json` + `fr.json`
- A11y: `<div onclick>` → `<button>`, `aria-label` on icon buttons, `alt` on images, visible focus rings
- Types: eliminate `any`, derive from API response types
