# Explore Map hero block — design

Date: 2026-10-16 · Status: approved (user: "continue")

## Goal

A new page-builder block type `explore_map` ("World Explorer") in the shared
`@japaconnect/landing-blocks` package that renders an interactive world-map
hero matching the "Explore the World. Plan Your Move." mockup: left legend /
region filter / quick-stats panel, headline + toolbar (View / Show / Currency
selects), a per-country choropleth world map with pin markers + flag tooltips,
zoom controls + compass, and a cost-of-living index legend bar.

## Decisions (user-approved)

- **Map rendering:** bundled TopoJSON-derived SVG paths (no Leaflet, no tiles,
  no runtime geo deps). A one-off generator emits
  `packages/landing-blocks/src/geo/worldMapData.ts` containing per-country
  `{ code, name, region, subregion, d }` path data projected to a
  1000×500 equirectangular viewBox, plus per-region bounding boxes for zoom.
- **Interactivity:** full. Region filter dims/highlights + zooms viewBox to
  region bounds; category legend + "Show" select filter pins by `pin.category`;
  "View" select switches pin source; currency select converts COL band labels;
  zoom +/- scale the viewBox; compass resets; pin hover shows a flag card;
  pin click navigates to `/spaces/{slug}`.
- **Data:** API + editable props. Pins default to country spaces
  (`GET /geo/countries`, top-N by member_count) with a manual `pins[]`
  override; region options from `GET /spaces/regions`; quick stats each bind
  a source (`countries` count, `col_cities` total, `blog` total, `services`
  total, or manual); currencies from `GET /fx/currencies`, conversion via
  `GET /fx/rates?base=` (feature-gated `fx.enabled` — hide select on failure).

## Known data gaps (documented, not faked)

- No country-level COL index endpoint exists. The map colors each country by
  its **region's** band via editable `region_bands` props — marketing-level
  approximation until a backend route lands (flagged in JEOS §16 catalog).
- `GeoSpace` projection omits `region`, so per-country region/subregion comes
  from the generated `worldMapData.ts` (static UN geography metadata), while
  pin filtering uses `GET /spaces?level=country&region=`.

## Props (`ExploreMapProps`)

- `headline`, `subtitle` (geo-copy compatible strings)
- `show_toolbar`, `show_legend`, `show_stats`, `show_region_filter`, `show_col_index`, `show_zoom`
- `views: [{ key, label, source: 'countries' | 'col_cities' | 'universities' }]`
- `categories: [{ key, icon, label, sublabel, color }]`
- `stats: [{ icon, label, source: 'manual' | 'countries' | 'col_cities' | 'blog' | 'services', value?, suffix? }]`
- `geo_source: 'api' | 'manual'`; `pins: [{ country, label?, tagline?, category?, href? }]`; `pin_limit`
- `col_bands: [{ key, label, min, max, color }]` (5 defaults)
- `region_bands: Record<string, bandKey>` (region → band)
- `default_currency` (`'USD'`)

## Files

- `packages/landing-blocks/src/geo/worldMapData.ts` — generated path + region data
- `scripts/gen-world-map.mjs` — generator (bun, fetches world-atlas + region meta once)
- `packages/landing-blocks/src/blocks/ExploreMapBlock.svelte` — block
- `packages/landing-blocks/src/editors/content/ExploreMapContentEditor.svelte` — inspector editor
- `packages/landing-blocks/src/types.ts` — `ExploreMapProps` + `explore_map` union member
- `packages/landing-blocks/src/render/blockRegistry.ts` + `editors/content/index.ts` — registrations
- `apps/admin/src/lib/utils/blocks.ts` — `buildBlock` defaults
- `.jeos/sections/16-page-builder.md` — catalog row (38 → 39)

## Testing

`bun run check` in `apps/main` and `apps/admin`; manual drag-in at
`http://localhost:3001/page-builder/pages/[id]`; verify endpoints degrade
gracefully (fx disabled → currency select hidden; geo API down → manual pins).
