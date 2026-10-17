# World Explorer (`explore_map`) Parent+Children Decomposition — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the monolithic `ExploreMapBlock.svelte` into an `explore_map` parent container block plus six registered child block types (`explore_map_toolbar`, `explore_map_legend`, `explore_map_region_filter`, `explore_map_stats`, `explore_map_canvas`, `explore_map_col_index`), following the `guest_*` precedent.

**Architecture:** Parent owns all config (`ExploreMapProps` + new `children?: Block[]`) and all interactive state via an `ExploreMapContext` class in `context.svelte.ts`, shared to children through Svelte `setContext`/`getContext`. Children are context-driven layout units with no own config. Spec: `docs/superpowers/specs/2026-10-17-explore-map-decomposition-design.md`.

**Tech Stack:** Svelte 5 runes, `@japaconnect/landing-blocks` workspace package, SvelteKit 2 admin SPA. Use `bun`/`bunx` only (Rule 18).

## Global Constraints

- Use `bun`/`bunx`/`bun run` — never npm/npx/pnpm/yarn (JEOS Rule 18).
- No AI attribution anywhere in code, commits, or docs (JEOS Rule 17).
- JEOS catalog updated before code (`.jeos/AGENTS.md` — "JEOS is always updated before code"). Task 1 does this.
- Commits go to `main`. No AI trailers in commit messages (project overrides default trailer convention — Rule 17).
- Refactor must be behavior-preserving: `/` homepage explorer and `/wiki/[space_slug]` (`WikiExploreMap`, all `show_*` off) render identically.
- Svelte 5 runes only — `$state`, `$derived`, `$effect`, `$props`. No Svelte 4 stores.

## Source file map (current monolith → destination)

All line numbers refer to `packages/landing-blocks/src/blocks/ExploreMapBlock.svelte` @ commit `eb15c442b` (790 lines).

| monolith lines | destination |
|---|---|
| 30–39 (interfaces `GeoSpaceLite`, `FxCurrency`, `Pin`) | `context.svelte.ts` (exported) |
| 41–58 (state fields) | `context.svelte.ts` class fields |
| 60–73 (init `$effect`) | stays in parent `ExploreMapBlock.svelte` |
| 75–133 (helpers: `headers`–`toPin`) | `context.svelte.ts` methods |
| 135–205 (fetch `$effect`s) | stays in parent `<script>` (writes ctx fields) |
| 207–244 (pins/regions deriveds) | `context.svelte.ts` `$derived` fields |
| 246–372 (zoom/pan/pointer) | `context.svelte.ts` fields + methods |
| 374–399 (currency/stat/icon helpers) | `context.svelte.ts` |
| 402–408 (section/aside shell + show flags) | parent template |
| 409–433 (legend card) | `ExploreMapLegendBlock.svelte` |
| 435–462 (region filter card) | `ExploreMapRegionFilterBlock.svelte` |
| 464–482 (stats card, left mode) | `ExploreMapStatsBlock.svelte` |
| 485–498 (COL card, bottom mode) | `ExploreMapColIndexBlock.svelte` (`layout==='card'` branch) |
| 504–552 (header row + toolbar) | header stays in parent; selects → `ExploreMapToolbarBlock.svelte` |
| 554–683 (map card incl. zoom/compass/floating stats) | `ExploreMapCanvasBlock.svelte` |
| 685–699 (COL bar, left mode) | `ExploreMapColIndexBlock.svelte` (`layout==='bar'` branch) |
| 705–730 (snippets `marker`, `pinCard`) | `ExploreMapCanvasBlock.svelte` |
| 732–790 (`.em-*` styles) | `.em-select*` → toolbar; `.em-card*` → canvas |

---

### Task 1: JEOS §16 catalog update

**Files:**
- Modify: `.jeos/sections/16-page-builder.md` (row at line ~35, plus new rows after it)

**Interfaces:**
- Produces: catalog contract for the six new type names used everywhere below: `explore_map_toolbar`, `explore_map_legend`, `explore_map_region_filter`, `explore_map_stats`, `explore_map_canvas`, `explore_map_col_index`.

- [ ] **Step 1: Update the `explore_map` row** — append `children[]` to its props and note the parent/children decomposition: `children[] of explore_map_* (toolbar, legend, region_filter, stats, canvas, col_index) — parent owns all config + shared interactive state via ExploreMapContext; children are context-driven layout units (CHILD_ONLY)`.

- [ ] **Step 2: Add six rows after the `explore_map` row**, same table format (`| \`type\` | category | label | props |`):

```markdown
| `explore_map_toolbar` | geo | Explorer Toolbar | (child-only; no props — reads parent context) View/Show/Currency selects |
| `explore_map_legend` | geo | Explorer Legend | (child-only; no props) category legend card, toggles pin category filter |
| `explore_map_region_filter` | geo | Explorer Region Filter | (child-only; no props) region list card, zooms/dims map |
| `explore_map_stats` | geo | Explorer Stats | (child-only; no props) quick-stats card; floats inside map when aside_position=bottom |
| `explore_map_canvas` | geo | Explorer Canvas | (child-only; no props) map card: country paths, pins, zoom controls, loading overlay |
| `explore_map_col_index` | geo | Explorer COL Index | (child-only; no props) cost-of-living band legend; bar variant under map, card variant in bottom aside |
```

- [ ] **Step 3: Commit**

```bash
git add .jeos/sections/16-page-builder.md
git commit -m "docs(jeos): explore_map decomposed into parent + explore_map_* children"
```

---

### Task 2: `ExploreMapContext` — shared state/behavior class

**Files:**
- Create: `packages/landing-blocks/src/blocks/explore-map/context.svelte.ts`

**Interfaces:**
- Consumes: `ExploreMapProps` from `../../types`; `FetchContext` from `../../data/types`; `countryCentroid` from `../../geo/countryCentroids`; `WORLD_COUNTRIES`, `WORLD_VIEWBOX`, `REGION_BOUNDS` from `../../geo/worldMapData`; `EXPLORE_MAP_DEFAULTS` from `../../blocks/exploreMapDefaults` (temporary path — Task 5 moves the file into this folder and updates the import to `./exploreMapDefaults`); `buildApiUrl` from `@japaconnect/api-url`; `Component` from `svelte`; `* as LucideIcons` from `lucide-svelte`.
- Produces (consumed by parent + all six children):
  - `EXPLORE_MAP_CTX: symbol`
  - `class ExploreMapContext` — constructor `new ExploreMapContext(args: { props: () => ExploreMapProps; fetchCtx: () => FetchContext | undefined; geoCountry: () => string | undefined })`
  - `getExploreMapCtx(): ExploreMapContext | undefined` — wraps `getContext`
  - `exploreMapChildVisible(ctx, type): boolean` — per-type render gate
  - `partitionExploreMapChildren(children: Block[], bottom: boolean): { aside: Block[]; main: Block[] }`
  - `defaultExploreMapChildren(id: string): Block[]`
  - exported interfaces `GeoSpaceLite`, `FxCurrency`, `Pin`

- [ ] **Step 1: Write the file.** Port the monolith script into a class. Full skeleton with the exact member list:

```ts
// Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
// Shared interactive state + data for the explore_map parent and its
// explore_map_* children. The parent instantiates this, setContext()s it,
// and runs the data-loading $effects; children consume it via getContext().
import { getContext, setContext } from 'svelte';
import type { Component } from 'svelte';
import type { Block, ExploreMapProps } from '../../types';
import type { FetchContext } from '../../data/types';
import { countryCentroid } from '../../geo/countryCentroids';
import { WORLD_COUNTRIES, WORLD_VIEWBOX, REGION_BOUNDS } from '../../geo/worldMapData';
import { EXPLORE_MAP_DEFAULTS } from '../../blocks/exploreMapDefaults';
import { buildApiUrl } from '@japaconnect/api-url';
import * as LucideIcons from 'lucide-svelte';

export const EXPLORE_MAP_CTX = Symbol('explore-map');

export interface GeoSpaceLite { /* lines 30-34 verbatim */ }
export interface FxCurrency { code: string; name: string; symbol: string }
export interface Pin { /* lines 36-39 verbatim */ }

const countryByCode = new Map(WORLD_COUNTRIES.map((c) => [c.code, c]));
const nameToCode = new Map(WORLD_COUNTRIES.map((c) => [c.name.toLowerCase(), c.code]));
const FALLBACK_FILL = '#ccdde8';
export const CARD_W = 150;
export const CARD_H = 60;
const VB_W = WORLD_VIEWBOX.w;
const VB_H = WORLD_VIEWBOX.h;
const reducedMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;

export class ExploreMapContext {
  constructor(private args: { props: () => ExploreMapProps; fetchCtx: () => FetchContext | undefined; geoCountry: () => string | undefined }) {}

  P = $derived({ ...EXPLORE_MAP_DEFAULTS, ...this.args.props() });

  // ── Remote data (written by the parent's $effects) ──
  apiCountries = $state<GeoSpaceLite[]>([]);
  apiRegions = $state<string[]>([]);
  currencies = $state<FxCurrency[]>([]);
  fxRates = $state<Record<string, number>>({});
  statCounts = $state<Record<string, number>>({});
  viewPins = $state<Pin[]>([]);
  loading = $state(true);

  // ── UI state ──
  selRegion = $state('');
  vbFit = $state(false);
  selCategory = $state('');
  selView = $state('');
  currency = $state('');
  vb = $state<[number, number, number, number]>([0, 0, VB_W, VB_H]);
  hoverIdx = $state(-1);
  svgEl = $state<SVGSVGElement | null>(null);
  panning = $state<{ px: number; py: number; vb: [number, number, number, number]; moved: boolean } | null>(null);
  justDragged = false;
  private tweenId = 0;

  // ── Fetch helpers (lines 75-93; fetchCtx via this.args.fetchCtx()) ──
  headers(): Record<string, string> { /* verbatim, read this.args.fetchCtx() */ }
  async api<T>(path: string): Promise<T | null> { /* verbatim */ }

  // ── Pin/format helpers (lines 95-133 verbatim → methods) ──
  flag(code: string): string {}
  centroid(code: string): [number, number] | null {}
  bandColorFor(region: string, subregion: string): string {}
  categoryColor(key?: string): string | null {}
  firstSentence(s?: string): string | undefined {}
  toPin(country, label, tagline?, category?, href?, pos?): Pin | null {}

  // ── Deriveds (lines 208-244 verbatim → $derived fields) ──
  basePins = $derived.by<Pin[]>(() => { /* lines 208-230 */ });
  visiblePins = $derived.by<Pin[]>(() => { /* lines 232-238 */ });
  regionOptions = $derived.by<string[]>(() => { /* lines 241-244 */ });

  // ── Zoom / pan (lines 247-372 verbatim → fields+methods) ──
  clampVb(v) {}
  setVb(target) {}
  zoomBy(f) {}
  fitBounds(b) {}
  fitRegion(region) {}
  selectRegion(name) {}
  resetView() {}
  pinScreen(p, rect) {}
  hoverAt(e, rect) {}
  onDown(e) {}
  onMove(e) {}
  onUp() {}
  onLeave() {}
  onClickCapture(e) {}

  // ── Currency / stats / icons (lines 374-399) ──
  rate = $derived(this.fxRates[this.currency] ?? 1);
  symbol = $derived(this.currencies.find((c) => c.code === this.currency)?.symbol ?? (this.currency === 'USD' ? '$' : ''));
  fmtMoney(n: number): string {}
  bandRange(min: number, max: number | null): string {}
  statValue(source: string, value?: string, suffix?: string): string {}
  iconFor(name?: string): Component | null {}
  visitorCode = $derived(this.args.geoCountry()?.toUpperCase() ?? '');
}

export function setExploreMapCtx(ctx: ExploreMapContext) { setContext(EXPLORE_MAP_CTX, ctx); }
export function getExploreMapCtx(): ExploreMapContext | undefined {
  return getContext<ExploreMapContext>(EXPLORE_MAP_CTX);
}
```

Port notes:
- Every bare identifier from the monolith (`selRegion`, `vb`, `visiblePins`, `P`, `fetchCtx`, `geoCountry`, …) becomes `this.*` / `this.args.*()`; module-scope maps/constants stay module-level.
- `vb`/`hoverIdx`/`svgEl`/`panning` live here even though only the canvas touches them — `selectRegion`/`zoomBy` write `vb`, so it must be shared.
- `headers()` reads `const fetchCtx = this.args.fetchCtx()`.

- [ ] **Step 2: Append the child helpers** (pure functions — also used by the parent's partition and each child's gate):

```ts
const ASIDE_LEFT = new Set(['explore_map_legend', 'explore_map_region_filter', 'explore_map_stats']);
const ASIDE_BOTTOM = new Set(['explore_map_legend', 'explore_map_region_filter', 'explore_map_col_index']);

/** Whether a child of `type` currently renders anything, given ctx config. */
export function exploreMapChildVisible(ctx: ExploreMapContext, type: string): boolean {
  const P = ctx.P;
  switch (type) {
    case 'explore_map_toolbar': return !!P.show_toolbar;
    case 'explore_map_legend': return !!P.show_legend && (P.categories?.length ?? 0) > 0;
    case 'explore_map_region_filter': return !!P.show_region_filter;
    // Bottom mode floats stats inside the canvas card instead.
    case 'explore_map_stats': return !!P.show_stats && (P.stats?.length ?? 0) > 0 && P.aside_position !== 'bottom';
    case 'explore_map_col_index': return !!P.show_col_index && (P.col_bands?.length ?? 0) > 0;
    case 'explore_map_canvas': return true;
    default: return true; // unknown nested types always render
  }
}

/** Split children into the aside vs main column by type (like guest_hero's rightTypes). */
export function partitionExploreMapChildren(children: Block[], bottom: boolean): { aside: Block[]; main: Block[] } {
  const set = bottom ? ASIDE_BOTTOM : ASIDE_LEFT;
  return {
    aside: children.filter((b) => set.has(b.type)),
    main: children.filter((b) => !set.has(b.type)),
  };
}

/** Seeded children for a block with no stored children — reproduces today's layout. */
export function defaultExploreMapChildren(id: string): Block[] {
  const mk = (type: string, suffix: string) =>
    ({ id: `${id}-${suffix}`, type, visible: true, props: {} }) as Block;
  return [
    mk('explore_map_toolbar', 'toolbar'),
    mk('explore_map_legend', 'legend'),
    mk('explore_map_region_filter', 'region'),
    mk('explore_map_stats', 'stats'),
    mk('explore_map_canvas', 'canvas'),
    mk('explore_map_col_index', 'colindex'),
  ];
}
```

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/blocks/explore-map/context.svelte.ts
git commit -m "feat(landing-blocks): ExploreMapContext — shared state for explore_map decomposition"
```

---

### Task 3: Six child block components

**Files:**
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapToolbarBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapLegendBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapRegionFilterBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapStatsBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapCanvasBlock.svelte`
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapColIndexBlock.svelte`

**Interfaces:**
- Consumes: `getExploreMapCtx`, `exploreMapChildVisible`, `Pin`, `CARD_W`, `CARD_H` from `./context.svelte.ts`; `LucideIcons` from `lucide-svelte`; `WORLD_COUNTRIES`, `WORLD_VIEWBOX` from `../../geo/worldMapData`.
- Produces: six components each taking `{ props: ExploreMapChildProps; fetchCtx?: FetchContext; geoCountry?: string; id?: string }` (declare the Props interface; only `ctx` is used — `props` is required so the registry call signature compiles).

- [ ] **Step 1: Shared script preamble for every child:**

```ts
<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  import type { ExploreMapChildProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible } from './context.svelte';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: ExploreMapChildProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
</script>
```

Wrap the whole template in `{#if ctx && exploreMapChildVisible(ctx, '<type>')} … {/if}` — children render nothing outside a parent or when their `show_*` gate is off.

- [ ] **Step 2: `ExploreMapToolbarBlock.svelte`** — port monolith lines 510–550 (the toolbar `div` with the three labels/selects) verbatim, replacing `P.` → `ctx.P.`, `selView`/`selCategory`/`currency` → `ctx.selView`/…, `currencies` → `ctx.currencies`. Drop the outer `{#if P.show_toolbar}` (the gate handles it). Move the `.em-select-wrap`/`.em-select`/`.em-chevron`/dark-mode styles (lines 733–764) into this file.

- [ ] **Step 3: `ExploreMapLegendBlock.svelte`** — port lines 409–432 (the legend card `div` incl. `<h3>` + `<ul>`), replacing `P.categories` → `ctx.P.categories`, `iconFor` → `ctx.iconFor`, `selCategory` → `ctx.selCategory`. Drop the outer `{#if}`.

- [ ] **Step 4: `ExploreMapRegionFilterBlock.svelte`** — port lines 436–461, replacing `resetView`→`ctx.resetView`, `selectRegion`→`ctx.selectRegion`, `selRegion`→`ctx.selRegion`, `regionOptions`→`ctx.regionOptions`.

- [ ] **Step 5: `ExploreMapStatsBlock.svelte`** — port lines 465–481 (the aside quick-stats card only; NOT the floating overlay). `ctx.P.stats`, `ctx.iconFor`, `ctx.statValue`. The `exploreMapChildVisible` gate already excludes bottom mode.

- [ ] **Step 6: `ExploreMapColIndexBlock.svelte`** — both COL variants chosen by `ctx.P.aside_position`:
  - `'bottom'` → card variant, port lines 486–497 (`rounded-2xl … p-4` card with vertical `<ul class="space-y-2.5">`)
  - else → bar variant, port lines 687–698 (`rounded-2xl … px-5 py-4` with `<ul class="flex flex-wrap …">`)
  - Replace `bandRange` → `ctx.bandRange`, `P.col_bands` → `ctx.P.col_bands`.

- [ ] **Step 7: `ExploreMapCanvasBlock.svelte`** — the map card. Script: the shared preamble + `bind:this={ctx.svgEl}` wiring. Port:
  - lines 555–560: map-card wrapper `div` + loading overlay (`ctx.loading`)
  - lines 561–633: `<svg>` — `ctx.vb`, `ctx.P.cover_mode`, `ctx.panning` cursor, all `on*` handlers → `ctx.onDown`/`ctx.onMove`/`ctx.onUp`/`ctx.onLeave`/`ctx.onClickCapture`, country paths (`WORLD_COUNTRIES` import stays), `ctx.bandColorFor`, `ctx.visitorCode`, `ctx.P.focus_d`/`ctx.P.container_d`, `ctx.visiblePins`
  - lines 635–660: zoom controls + compass (`ctx.P.show_zoom`, `ctx.zoomBy`, `ctx.resetView`)
  - lines 662–682: bottom-mode floating stats overlay (unchanged gate: `ctx.P.aside_position === 'bottom' && ctx.P.show_stats && …`)
  - lines 705–730: `marker` + `pinCard` snippets verbatim (`vb`→`ctx.vb`, `CARD_W`/`CARD_H` imported from `./context.svelte.ts`)
  - lines 765–789: `.em-card*` styles into this file's `<style>`

- [ ] **Step 8: Commit**

```bash
git add packages/landing-blocks/src/blocks/explore-map/
git commit -m "feat(landing-blocks): explore_map_* child components — toolbar, legend, region filter, stats, canvas, col index"
```

---

### Task 4: Register the six child types

**Files:**
- Modify: `packages/landing-blocks/src/types.ts` (~line 479 `ExploreMapProps`, ~line 1124 union)
- Modify: `packages/landing-blocks/src/render/blockRegistry.ts`
- Modify: `packages/landing-blocks/src/render/blockVisibility.ts` (line ~23 `dynamicBlockTypes`)
- Create: `packages/landing-blocks/src/editors/content/ExploreMapChildContentEditor.svelte`
- Modify: `packages/landing-blocks/src/editors/content/index.ts`

**Interfaces:**
- Consumes: the six components from Task 3.
- Produces: `ExploreMapChildProps` + six named aliases; union members `explore_map_toolbar | explore_map_legend | explore_map_region_filter | explore_map_stats | explore_map_canvas | explore_map_col_index`; registry labels "Explorer Toolbar" etc., `category: 'geo'`.

- [ ] **Step 1: `types.ts`** — add `children?: Block[];` at the end of `ExploreMapProps` (after `aside_position`), with comment `/** Nested explore_map_* child blocks; empty/unset → seeded defaults render the standard layout. */`. Then add after the `ExploreMapProps` interface:

```ts
/** Shared props for `explore_map_*` children — config lives on the parent
 *  `explore_map` block; children are context-driven layout units. */
export interface ExploreMapChildProps {
  /** Reserved for future per-child overrides — unused today. */
  overrides?: Record<string, unknown>;
}
export type ExploreMapToolbarProps = ExploreMapChildProps;
export type ExploreMapLegendProps = ExploreMapChildProps;
export type ExploreMapRegionFilterProps = ExploreMapChildProps;
export type ExploreMapStatsProps = ExploreMapChildProps;
export type ExploreMapCanvasProps = ExploreMapChildProps;
export type ExploreMapColIndexProps = ExploreMapChildProps;
```

Add six union members after the `explore_map` member:

```ts
  | (BaseBlock & { type: 'explore_map_toolbar'; props: ExploreMapToolbarProps })
  | (BaseBlock & { type: 'explore_map_legend'; props: ExploreMapLegendProps })
  | (BaseBlock & { type: 'explore_map_region_filter'; props: ExploreMapRegionFilterProps })
  | (BaseBlock & { type: 'explore_map_stats'; props: ExploreMapStatsProps })
  | (BaseBlock & { type: 'explore_map_canvas'; props: ExploreMapCanvasProps })
  | (BaseBlock & { type: 'explore_map_col_index'; props: ExploreMapColIndexProps })
```

- [ ] **Step 2: `blockRegistry.ts`** — six imports + six entries after `explore_map`:

```ts
import ExploreMapToolbarBlock from '../blocks/explore-map/ExploreMapToolbarBlock.svelte';
import ExploreMapLegendBlock from '../blocks/explore-map/ExploreMapLegendBlock.svelte';
import ExploreMapRegionFilterBlock from '../blocks/explore-map/ExploreMapRegionFilterBlock.svelte';
import ExploreMapStatsBlock from '../blocks/explore-map/ExploreMapStatsBlock.svelte';
import ExploreMapCanvasBlock from '../blocks/explore-map/ExploreMapCanvasBlock.svelte';
import ExploreMapColIndexBlock from '../blocks/explore-map/ExploreMapColIndexBlock.svelte';
```

```ts
  explore_map_toolbar: { component: ExploreMapToolbarBlock, icon: 'ListFilter', label: 'Explorer Toolbar', category: 'geo' },
  explore_map_legend: { component: ExploreMapLegendBlock, icon: 'List', label: 'Explorer Legend', category: 'geo' },
  explore_map_region_filter: { component: ExploreMapRegionFilterBlock, icon: 'MapPin', label: 'Explorer Region Filter', category: 'geo' },
  explore_map_stats: { component: ExploreMapStatsBlock, icon: 'BarChart', label: 'Explorer Stats', category: 'geo' },
  explore_map_canvas: { component: ExploreMapCanvasBlock, icon: 'Globe2', label: 'Explorer Canvas', category: 'geo' },
  explore_map_col_index: { component: ExploreMapColIndexBlock, icon: 'DollarSign', label: 'Explorer COL Index', category: 'geo' },
```

- [ ] **Step 3: `blockVisibility.ts`** — extend `dynamicBlockTypes` to include all six child types (context-driven like `explore_map`; keeps `hide_if_empty`/`initialHasContent` correct for empty-props children):

```ts
const dynamicBlockTypes = new Set<string>(['widget', 'entity_collection', 'entity_feature', 'live_rate_ticker', 'explore_map', 'explore_map_toolbar', 'explore_map_legend', 'explore_map_region_filter', 'explore_map_stats', 'explore_map_canvas', 'explore_map_col_index']);
```

- [ ] **Step 4: `ExploreMapChildContentEditor.svelte`** — shared note editor (every registered type needs an editor entry):

```svelte
<script lang="ts">
  import type { ExploreMapChildProps } from '../../types';
  interface Props { props: ExploreMapChildProps; onchange: (v: ExploreMapChildProps) => void }
  let { props }: Props = $props();
</script>

<p class="text-xs text-[var(--color-muted)]">
  This section is part of the World Explorer block. Its content and data are
  configured on the parent block — use the sidebar to reorder, hide, or style
  it. Visibility, styles and motion settings apply per section.
</p>
```

- [ ] **Step 5: `editors/content/index.ts`** — import + six entries:

```ts
explore_map_toolbar: ExploreMapChildContentEditor,
explore_map_legend: ExploreMapChildContentEditor,
explore_map_region_filter: ExploreMapChildContentEditor,
explore_map_stats: ExploreMapChildContentEditor,
explore_map_canvas: ExploreMapChildContentEditor,
explore_map_col_index: ExploreMapChildContentEditor,
```

- [ ] **Step 6: Type-check**

Run: `cd apps/main && bun run check`
Expected: 0 errors (the old monolith still renders — children just exist now; `Record<Block['type'], …>` registries stay total).

- [ ] **Step 7: Commit**

```bash
git add packages/landing-blocks/src/types.ts packages/landing-blocks/src/render/ packages/landing-blocks/src/editors/content/
git commit -m "feat(landing-blocks): register explore_map_* child block types"
```

---

### Task 5: Parent rewrite — `ExploreMapBlock` becomes the container

**Files:**
- Create: `packages/landing-blocks/src/blocks/explore-map/ExploreMapBlock.svelte`
- Move: `packages/landing-blocks/src/blocks/exploreMapDefaults.ts` → `packages/landing-blocks/src/blocks/explore-map/exploreMapDefaults.ts` (fix its `../types` import → `../../types`)
- Delete: `packages/landing-blocks/src/blocks/ExploreMapBlock.svelte`, `packages/landing-blocks/src/blocks/exploreMapDefaults.ts`
- Modify: `packages/landing-blocks/src/index.ts` (lines 36, 40 — new paths)
- Modify: `packages/landing-blocks/src/render/blockRegistry.ts` (ExploreMapBlock import path)
- Modify: `packages/landing-blocks/src/blocks/explore-map/context.svelte.ts` (defaults import → `./exploreMapDefaults`)
- Modify: `packages/landing-blocks/src/editors/content/ExploreMapContentEditor.svelte` (fix its `exploreMapDefaults` import path)

**Interfaces:**
- Consumes: `ExploreMapContext`, `setExploreMapCtx`, `partitionExploreMapChildren`, `defaultExploreMapChildren`, `exploreMapChildVisible` from `./context.svelte.ts`; `blockRegistry`, `BlockShell`, `isBlockVisible`, `isBlockEmpty`, `responsiveVisibilityCss`, `ScopedCss` (same set `GuestHeroContainerBlock` uses).
- Produces: same exported default component signature — `{ props: ExploreMapProps; fetchCtx?: FetchContext; geoCountry?: string; id?: string }` — so `WikiExploreMap` and the registry work unchanged.

- [ ] **Step 1: Write the new parent.** Script = ctx instantiation + `setContext` + the four `$effect`s from the monolith (verbatim, `ctx.*`), then partition logic:

```svelte
<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  import type { Block, ExploreMapProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ExploreMapContext, setExploreMapCtx, partitionExploreMapChildren,
           defaultExploreMapChildren, exploreMapChildVisible } from './context.svelte';
  import { blockRegistry } from '../../render/blockRegistry';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import { responsiveVisibilityCss } from '../../shell/styleToCss';
  import ScopedCss from '../../shell/ScopedCss.svelte';
  import BlockShell from '../../shell/BlockShell.svelte';

  interface Props {
    props: ExploreMapProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const ctx = new ExploreMapContext({
    props: () => props,
    fetchCtx: () => fetchCtx,
    geoCountry: () => geoCountry,
  });
  setExploreMapCtx(ctx);

  // ── $effects ported verbatim from the monolith (identifiers → ctx.*) ──
  // 1. init selections (monolith lines 61-73)
  // 2. mount fetch: countries/regions/stats (lines 138-161)
  // 3. fx fetch keyed on ctx.P.default_currency (lines 164-174)
  // 4. viewPins fetch keyed on ctx.selView (lines 177-205)

  const children = $derived(props.children?.length ? props.children : defaultExploreMapChildren(id ?? 'explore-map'));
  const bottom = $derived(ctx.P.aside_position === 'bottom');
  const parts = $derived(partitionExploreMapChildren(children, bottom));
  const visibleAside = $derived(parts.aside.filter((b) => isBlockVisible(b, geoCountry) && exploreMapChildVisible(ctx, b.type)));
  const visibleMain = $derived(parts.main.filter((b) => isBlockVisible(b, geoCountry)));
  const visibilityCss = $derived(responsiveVisibilityCss(children));
</script>
```

- [ ] **Step 2: Template** — port monolith lines 402–703 keeping identical wrapper classes/order, but replace the fixed aside panels and main-column contents with region loops (mirroring `GuestHeroContainerBlock`'s `renderableComponent` + `BlockShell` pattern):

```svelte
<ScopedCss css={visibilityCss} />

<section class="explore-map {ctx.P.cover_mode ? '' : 'px-4 py-10 sm:px-6'}" id={id}>
  <div class="mx-auto max-w-7xl">
    <div class="flex flex-col gap-5 {bottom ? '' : 'lg:flex-row'}">
      {#if visibleAside.length > 0}
        <aside class="order-2 w-full shrink-0 {bottom ? 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3' : 'flex flex-col gap-4 lg:order-1 lg:w-60'}">
          {#each visibleAside as block (block.id)}
            {@render childShell(block)}
          {/each}
        </aside>
      {/if}
      <div class="order-1 min-w-0 flex-1 {bottom ? '' : 'lg:order-2'}">
        <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            {#if ctx.P.headline}<h2 class="text-2xl font-bold tracking-tight text-[var(--color-text)] sm:text-[28px]">{ctx.P.headline}</h2>{/if}
            {#if ctx.P.subtitle}<p class="mt-1 text-sm text-[var(--color-muted)]">{ctx.P.subtitle}</p>{/if}
          </div>
          <!-- toolbar child renders here via the main partition, FIRST -->
        </div>
        {#each visibleMain as block (block.id)}
          {@render childShell(block)}
        {/each}
      </div>
    </div>
  </div>
</section>

{#snippet childShell(block: Block)}
  {@const meta = blockRegistry[block.type]}
  {#if meta}
    {@const Component = meta.component}
    <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
      <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
    </BlockShell>
  {/if}
{/snippet}
```

**Positioning caveat:** the toolbar must sit inside the header row beside the headline (today it's in that flex row), and the canvas/col_index follow below. Handle by splitting `visibleMain` at render: render `explore_map_toolbar`-typed children inside the header div's right slot, and the rest below:

```ts
const mainToolbar = $derived(visibleMain.filter((b) => b.type === 'explore_map_toolbar'));
const mainBody = $derived(visibleMain.filter((b) => b.type !== 'explore_map_toolbar'));
```

```svelte
<div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
  <div class="min-w-0">…headline/subtitle…</div>
  {#each mainToolbar as block (block.id)}{@render childShell(block)}{/each}
</div>
{#each mainBody as block (block.id)}{@render childShell(block)}{/each}
```

- [ ] **Step 3: Move defaults + update imports**

```bash
mkdir -p packages/landing-blocks/src/blocks/explore-map
git mv packages/landing-blocks/src/blocks/exploreMapDefaults.ts packages/landing-blocks/src/blocks/explore-map/exploreMapDefaults.ts
git rm packages/landing-blocks/src/blocks/ExploreMapBlock.svelte
```

Update import paths:
- `exploreMapDefaults.ts`: `import type { ExploreMapProps } from '../types';` → `'../../types'`
- `context.svelte.ts`: `'../../blocks/exploreMapDefaults'` → `'./exploreMapDefaults'`
- `src/index.ts` line 36: `'./blocks/exploreMapDefaults'` → `'./blocks/explore-map/exploreMapDefaults'`; line 40: `'./blocks/ExploreMapBlock.svelte'` → `'./blocks/explore-map/ExploreMapBlock.svelte'`
- `blockRegistry.ts`: `import ExploreMapBlock from '../blocks/ExploreMapBlock.svelte'` → `'../blocks/explore-map/ExploreMapBlock.svelte'`
- `ExploreMapContentEditor.svelte`: find its `exploreMapDefaults` import and repoint to `../blocks/explore-map/exploreMapDefaults`

- [ ] **Step 4: Type-check**

Run: `cd apps/main && bun run check`
Expected: 0 errors.

- [ ] **Step 5: Commit**

```bash
git add -A packages/landing-blocks/src
git commit -m "refactor(landing-blocks): explore_map renders via parent container + explore_map_* children"
```

---

### Task 6: Admin page-builder wiring

**Files:**
- Modify: `apps/admin/src/lib/utils/blocks.ts` (`CONTAINER_LIKE_TYPES` ~line 5, `CHILD_ONLY_TYPES` ~line 33, `defaultChildrenFor` ~line 265)

**Interfaces:**
- Consumes: `defaultExploreMapChildren` semantics mirrored locally (the admin file builds blocks with `crypto.randomUUID()` + `props: {}` — it does not import the package helper; keep it consistent with the package's seed list).

- [ ] **Step 1: `CONTAINER_LIKE_TYPES`** — add `'explore_map'` (after `'container'` line is fine). This makes `childSlotsOf`/`mapChildSlots`/`seedBlockDefaults`/`isContainerLike` treat `props.children` as its slot automatically.

- [ ] **Step 2: `CHILD_ONLY_TYPES`** — add the six:

```ts
	'explore_map_toolbar',
	'explore_map_legend',
	'explore_map_region_filter',
	'explore_map_stats',
	'explore_map_canvas',
	'explore_map_col_index',
```

Also update the palette message? — `isChildOnlyType` already shows a generic "only works inside a container" toast; the string `pageBuilder.child_only_block` mentions "guest container" — acceptable, no change needed (i18n key already generic enough in practice).

- [ ] **Step 3: `defaultChildrenFor`** — add a case before `default:`:

```ts
		case 'explore_map':
			return [
				{ id: crypto.randomUUID(), type: 'explore_map_toolbar', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'explore_map_legend', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'explore_map_region_filter', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'explore_map_stats', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'explore_map_canvas', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'explore_map_col_index', visible: true, props: {} },
			] as Block[];
```

(No `buildBlock` change needed — the existing `explore_map` branch seeds `EXPLORE_MAP_DEFAULTS`, and `makeBlock` → `seedBlockDefaults` adds children now that it's container-like.)

- [ ] **Step 4: Type-check**

Run: `cd apps/admin && bun run check`
Expected: 0 errors.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/utils/blocks.ts
git commit -m "feat(admin): explore_map container-like; explore_map_* children seeded + child-only"
```

---

### Task 7: Unit test — partition + default children

**Files:**
- Create: `packages/landing-blocks/src/blocks/explore-map/exploreMapChildren.test.ts`

**Interfaces:**
- Consumes: `partitionExploreMapChildren`, `defaultExploreMapChildren` from `./context.svelte.ts`. NOTE: importing context.svelte.ts pulls in `lucide-svelte`/geo data — fine for vitest (node env, no DOM needed for these pure functions; `reducedMotion` guards on `typeof window`).

- [ ] **Step 1: Write the test** (mirrors `guestChildren.test.ts` style — vitest):

```ts
import { describe, it, expect } from 'vitest';
import { partitionExploreMapChildren, defaultExploreMapChildren } from './context.svelte';
import type { Block } from '../../types';

const kids = defaultExploreMapChildren('t');
const custom: Block = { id: 'x', type: 'cta_button', visible: true, props: {} } as Block;

describe('partitionExploreMapChildren', () => {
  it('left mode: legend/filter/stats aside; toolbar/canvas/col_index main', () => {
    const { aside, main } = partitionExploreMapChildren(kids, false);
    expect(aside.map((b) => b.type)).toEqual(['explore_map_legend', 'explore_map_region_filter', 'explore_map_stats']);
    expect(main.map((b) => b.type)).toEqual(['explore_map_toolbar', 'explore_map_canvas', 'explore_map_col_index']);
  });
  it('bottom mode: col_index joins the aside grid; stats leaves it', () => {
    const { aside, main } = partitionExploreMapChildren(kids, true);
    expect(aside.map((b) => b.type)).toEqual(['explore_map_legend', 'explore_map_region_filter', 'explore_map_col_index']);
    expect(main.map((b) => b.type)).toEqual(['explore_map_toolbar', 'explore_map_canvas']);
  });
  it('unknown types go to main, preserving order', () => {
    const { main } = partitionExploreMapChildren([...kids, custom], false);
    expect(main.at(-1)).toBe(custom);
  });
});

describe('defaultExploreMapChildren', () => {
  it('seeds the six children with stable ids', () => {
    expect(kids.map((b) => b.id)).toEqual(['t-toolbar', 't-legend', 't-region', 't-stats', 't-canvas', 't-colindex']);
  });
});
```

- [ ] **Step 2: Run it**

Run: `cd packages/landing-blocks && bunx vitest run`
Expected: all tests pass (existing `guestChildren.test.ts` included).

- [ ] **Step 3: Commit**

```bash
git add packages/landing-blocks/src/blocks/explore-map/exploreMapChildren.test.ts
git commit -m "test(landing-blocks): explore_map child partitioning + seed defaults"
```

---

### Task 8: Full verification

- [ ] **Step 1: `cd apps/main && bun run check`** — 0 errors.
- [ ] **Step 2: `cd apps/admin && bun run check`** — 0 errors.
- [ ] **Step 3: `cd packages/landing-blocks && bunx vitest run`** — all pass.
- [ ] **Step 4: Manual smoke (if dev servers available):** homepage `/` explorer renders with legend/filter/stats/toolbar/map/COL bar; `/wiki/[space_slug]` map-only render unchanged (all chrome off); admin page-builder: drag "World Explorer" in → six children in sidebar tree, reorder/hide works, children rejected at top level.
- [ ] **Step 5: Final commit** (if any fixes were needed).
