# World Explorer (`explore_map`) — parent + children decomposition

Date: 2026-10-17 · Status: approved (user-selected: page-builder block types, parent owns config)

## Goal

Decompose the monolithic ~790-line `ExploreMapBlock.svelte` into a page-builder
parent/children structure following the `guest_*` precedent: `explore_map`
becomes a container block with `children: Block[]`, and six new child block
types render the parts (toolbar, legend, region filter, stats, canvas, COL
index). Admins gain per-part reorder, hide, style, motion, and responsive
visibility via `BlockShell` — for free.

Existing saved pages and `WikiExploreMap` (which mounts `ExploreMapBlock`
directly with flat props) render pixel-identical.

## Config model (user-approved)

**Parent owns config.** `ExploreMapProps` is unchanged plus `children?: Block[]`.
All data props (`views`, `categories`, `stats`, `pins`, `col_bands`,
`region_bands`, `geo_source`, `default_currency`, `show_*`, `aside_position`,
`cover_mode`, `initial_*`, `focus_d`, `container_d`) stay on the parent; the
existing `ExploreMapContentEditor` remains the single inspector. Children are
layout units — they carry no config props.

## Shared state: `ExploreMapContext`

The children are deeply interactive with each other (region filter zooms the
map; category/show selects filter pins; currency rewrites COL band labels), so
a single context object owns ALL runtime state and data fetching:

- New file `blocks/explore-map/context.svelte.ts` exporting an
  `ExploreMapContext` class with `$state`/`$derived` fields — essentially
  today's entire `<script>` moved wholesale: selections (`selRegion`,
  `selCategory`, `selView`, `currency`), viewBox (`vb`) + zoom/pan handlers,
  `hoverIdx`, fetched data (`apiCountries`, `apiRegions`, `currencies`,
  `fxRates`, `statCounts`, `viewPins`), derived `visiblePins`/`regionOptions`,
  and helpers (`selectRegion`, `resetView`, `zoomBy`, `statValue`, `iconFor`,
  `bandColorFor`, `bandRange`, `pinScreen`, pointer handlers).
- Parent instantiates it and `setContext`s it; children `getContext` it.
  A child rendered without a parent context renders nothing (they are
  `CHILD_ONLY_TYPES` — unreachable at top level, defensive anyway).
- The `P` merged-props derived moves into the context so children read
  `ctx.P.*`.

## Parent (`explore_map`)

- Keeps rendering the `<section>` shell, max-width wrapper, and the
  headline/subtitle row.
- Partitions children into regions by type (like `guest_hero`'s `rightTypes`):

  | region | `aside_position: 'left'` (default) | `aside_position: 'bottom'` |
  |---|---|---|
  | aside | legend, region_filter, stats | legend, region_filter, col_index (grid below) |
  | main | toolbar, canvas, col_index | toolbar, canvas |

- Unknown nested types append to the main column — no child silently dropped
  (same rule as `guest_hero`).
- When `props.children` is empty/unset, renders a seeded default child list
  (same trick as `GuestHeroContainerBlock.defaultChildren`) → identical
  output to today.
- Each child renders through `blockRegistry` + `BlockShell` with
  `isBlockVisible` / `responsiveVisibilityCss`, matching `GuestHeroContainerBlock`.

## Children (all `CHILD_ONLY_TYPES`, no own props)

| type | component | renders |
|---|---|---|
| `explore_map_toolbar` | `ExploreMapToolbarBlock` | View / Show / Currency selects bound to ctx |
| `explore_map_legend` | `ExploreMapLegendBlock` | categories card; toggles `ctx.selCategory` |
| `explore_map_region_filter` | `ExploreMapRegionFilterBlock` | regions card; calls `ctx.selectRegion`/`resetView` |
| `explore_map_stats` | `ExploreMapStatsBlock` | quick-stats card from `ctx.statCounts` |
| `explore_map_canvas` | `ExploreMapCanvasBlock` | map card: loading overlay, SVG countries, `focus_d`/`container_d` shapes, pins + pin cards, zoom controls + compass, floating stats overlay |
| `explore_map_col_index` | `ExploreMapColIndexBlock` | COL band legend — bar variant (left mode) or card variant (bottom mode), chosen from `ctx.P.aside_position` |

Each child also checks its `show_*` flag on `ctx.P` (`show_toolbar`,
`show_legend`, `show_region_filter`, `show_stats`, `show_col_index`,
`show_zoom` — the last inside canvas) so seeded children in wiki mode
(all flags off) produce zero chrome.

### Fidelity details

- **Bottom-mode floating stats:** today the stats card floats bottom-left
  inside the map card when `aside_position === 'bottom'`. The canvas keeps
  owning that overlay; `explore_map_stats` renders its standalone card only
  when `aside_position !== 'bottom'` — no duplication, identical layout.
- **`explore_map_stats` + `explore_map_col_index`** have two visual variants
  selected by context — matching today's aside/bottom markup branches.
- **Snippets** (`marker`, `pinCard`) and the `.em-*` styles move into the
  canvas child (they're only used by the SVG).
- **`.em-select-*` styles** move into the toolbar child.

## Files

- `packages/landing-blocks/src/blocks/explore-map/` — new folder:
  `ExploreMapBlock.svelte` (moved), `context.svelte.ts`, the six child
  components, `exploreMapDefaults.ts` (moved). Old
  `blocks/ExploreMapBlock.svelte` / `blocks/exploreMapDefaults.ts` deleted.
- `packages/landing-blocks/src/index.ts` — update the `ExploreMapBlock` /
  `EXPLORE_MAP_DEFAULTS` export paths to the new folder.
- `packages/landing-blocks/src/types.ts` — `children?: Block[]` on
  `ExploreMapProps`; 6 new `Block` union members; empty
  `ExploreMapToolbarProps`/`…Legend`/`…RegionFilter`/`…Stats`/`…Canvas`/`…ColIndex`
  interfaces (empty — they carry no config today; the named types keep the
  union's one-props-type-per-member convention and leave room for future
  per-child overrides).
- `packages/landing-blocks/src/render/blockRegistry.ts` — +6 entries,
  `category: 'geo'`, icons `Globe2`-family.
- `packages/landing-blocks/src/render/blockVisibility.ts` — add the six child
  types to `dynamicBlockTypes` so empty-props children are never treated as
  "empty" (context-driven, like `explore_map` itself).
- `packages/landing-blocks/src/editors/content/index.ts` — +6 entries → one
  shared `ExploreMapChildContentEditor` ("Configured on the parent World
  Explorer block" note; registry is `Record<Block['type'], …>` so entries are
  required).
- `apps/admin/src/lib/utils/blocks.ts` — `CONTAINER_LIKE_TYPES += explore_map`;
  `CHILD_ONLY_TYPES +=` the six; `defaultChildrenFor('explore_map')` seeds the
  six children in order `[toolbar, legend, region_filter, stats, canvas,
  col_index]` (partitioning is by type, but within each region the seed order
  reproduces today's top-to-bottom arrangement).
- `.jeos/sections/16-page-builder.md` — catalog: update `explore_map` row,
  add six child rows.

## Testing

- `bun run check` (or repo equivalent) in `packages/landing-blocks`,
  `apps/main`, `apps/admin`.
- Regression: `/` homepage explore_map and `/wiki/[space_slug]`
  (`WikiExploreMap`, all chrome off) render identically.
- Admin: drag World Explorer into a page → six seeded children visible in the
  sidebar tree; reorder/hide a child works; children can't be added top-level.

## Non-goals

- Per-child config overrides (child props) — explicitly deferred; context
  supplies all config from the parent.
- Moving data fetching into children — stays in the parent context.
- Backend changes — none.
