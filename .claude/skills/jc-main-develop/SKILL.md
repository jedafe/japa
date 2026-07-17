---
name: jc-main-develop
description: Use when building new pages or components for the JapaConnect Main App (apps/web, port 3000, SSR). Covers SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind v4 + LayerChart, dev.to/Reddit/Ghost/x.com design rules, the API-Readiness Gate, and Rule 16.
---

# JapaConnect Main App — Development Skill

You are building JapaConnect's public-facing community platform. SSR via adapter-node + Bun, port 3000.

## ⛔ API-READINESS GATE — RUN BEFORE ANY IMPLEMENTATION
Before writing a single line for any page or component:
1. **List every API endpoint** the feature will consume (reads + mutations).
2. **Verify each exists** — check the JEOS `api-routes-catalog.md`, or probe the running backend (`curl -s http://localhost:8080/api/v1/...`).
3. **If ANY endpoint is missing or returns 404:** STOP. Do not implement. Do not mock. Do not hardcode placeholder data. Output an alert in this exact format and wait for the user:

```
⚠️ MISSING BACKEND API ROUTES — implementation paused
Feature: [feature name]
Missing routes:
  - GET  /api/v1/...
  - POST /api/v1/...
Action needed: implement these routes in the Go backend before frontend work continues.
```

Never work around a missing endpoint. Rule 16 (no hardcoded data) makes mocking a violation, not a shortcut.

## Non-Negotiable Stack
- SvelteKit 2.x, adapter-node, Bun runtime · Svelte 5 **strict Runes** (`$state`, `$derived`, `$props`, `$effect`)
- Bits UI (latest stable), dot-notation (`<Dialog.Root>`, `<Select.Trigger>`) for ALL interactive primitives
- Tailwind CSS v4 — utilities via `class` prop; theme via CSS custom properties; dark mode = `[data-theme="dark"]` from `jc_theme` cookie (never Tailwind `dark:`)
- **LayerChart** for ALL data visualization (see below) · Lucide Svelte icons (tree-shaken) · TypeScript, zero `any`
- Forbidden: `export let`, `$:`, `on:click`, `createEventDispatcher`, localStorage auth, AI attribution in comments/commits

## LayerChart — Data Visualization Standard
All charts on user dashboards, profile analytics tabs, and org analytics use **LayerChart** (Svelte-native, built on Layer Cake — install: `bun add layerchart`).

```svelte
<script lang="ts">
  import { Chart, Svg, Axis, Spline, Area, Bars, Tooltip } from 'layerchart';
  import { scaleTime, scaleLinear } from 'd3-scale';
  let { data }: { data: { date: Date; views: number }[] } = $props();
</script>

<div class="h-64 w-full">
  <Chart {data} x="date" xScale={scaleTime()} y="views" yScale={scaleLinear()} yNice padding={{ left: 32, bottom: 24 }} tooltip>
    <Svg>
      <Axis placement="left" grid rule />
      <Axis placement="bottom" rule />
      <Area class="fill-[var(--color-primary)]/20" />
      <Spline class="stroke-[var(--color-primary)] stroke-2" />
    </Svg>
    <Tooltip.Root let:data>
      <Tooltip.Header>{data.date.toLocaleDateString()}</Tooltip.Header>
      <Tooltip.List><Tooltip.Item label="Views" value={data.views} /></Tooltip.List>
    </Tooltip.Root>
  </Chart>
</div>
```

Rules: chart colors ONLY via CSS custom properties (dark-mode safe) · chart data ONLY from API (Rule 16) · every chart wrapped in a fixed-height container · every chart has a skeleton loading state · sparklines on stat cards use a minimal `<Chart>` with `<Spline>` only, no axes.

## API Contract (Rule 16)
- All data from `/api/v1` — `+page.server.ts` `load()` fetches from API, never returns static data
- Mutations: `fetch` + CSRF token from `$page.data.csrfToken` · Auth from `$page.data.session`
- Lists: `{ data: T[], meta: { total, page, limit, next_cursor? } }` · Errors: `throw error(status, message)`

## Rendering Strategy
SSR for all indexable routes (`/`, `/forum/*`, `/blog/*`, `/spaces/*`, `/u/*`, `/org/*`) · CSR for auth-only (`/home`, `/dms`, `/notifications`) · Optimistic UI + rollback for reactions/bookmarks/follows · skeletons for every async section · `data-sveltekit-preload-data="hover"` already on `<body>`.

## Design Rules (dev.to / Reddit / Ghost)
0. **Homepage `/`:** hero banner (welcome + CTA) → tabbed latest content (one tab per post type from API + Spaces + Comparisons) → Explore button → trending sidebar
1. **Feeds/timeline/Explore for login users feeds, space feeds, organization feeds, etc:** should mimic https://x.com/explore expecially top tab and sidebars
2. **Blog single:** dev.to — sticky left action sidebar (desktop), bottom bar (mobile), right sidebar (TOC + related), TipTap HTML in `.prose`
3. **Forum thread:** Reddit — vote rail, nested threaded replies, Best/Top/New/Controversial tabs
4. **Blog listing:** Ghost — magazine cards, feature image, reading time
5. **Profiles/dashboards/settings/analytics:** dev.to (analytics tab uses LayerChart)
6. **Spaces/Tags/Users/Orgs listings:** dev.to tags page — search bar + filters + sorting + tab switcher between entity types + Follow/Unfollow per card
7. **Content create page:** The page should get inspiration from https://dev.to/new but with TipTap editor
8. Palette: `--color-bg #FAFAFA/#0A0A0A`, `--color-primary #3B49DF`, `--color-surface`, `--color-muted`, `--color-border` — all as CSS custom properties

## TipTap Triggers (Locked)
`/` slash commands · `@` mentions · `#` tags · `[[` interlinking

## i18n
Every user-facing string: `$t('key')`; add keys to `en.json` AND `fr.json`.

## Deliverables Per Task
1. API-Readiness Gate output (endpoints verified) · 2. `+page.server.ts` typed `load()` · 3. `+page.svelte` (Runes, Bits UI, skeletons, dark mode) · 4. sub-components in `$lib/components/` · 5. types in `$lib/types/index.ts`
