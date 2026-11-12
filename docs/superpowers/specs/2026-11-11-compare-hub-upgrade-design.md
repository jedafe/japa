# Compare Hub Upgrade — Discovery-First IA

Date: 2026-11-11
Status: Approved design, pre-planning
Surfaces: `apps/main` (`/compare`, `/compare/*`, `/comparisons`, `/comparisons/*`), Go backend (`internal/comparison`, `internal/db/queries/comparison.sql`)

## 1. Goal

Restructure the comparison hub from **tool-first** (pick a type → pick items → generate) to **discovery-first** (browse categories → trending comparisons → start custom comparison), matching the approved mockup:

- `/compare` becomes a marketing-style landing: hero, "What would you like to compare?" search, category card grid, trending comparisons, Japa AI help card.
- `/compare/<family>` becomes a browsable directory of published comparisons for a type family, with a refine sidebar and a "Start custom comparison" CTA that hands off into the existing picker.

**Hard constraint:** all existing picker/workspace logic is preserved unchanged — scope chains (`NarrowPanel`), `CompareTray`, `FieldPoolPicker`, `StepHeader` behavior, `preselect`/`compare_token`, guest-selection login flow, promote/delete, AI quick compare, `mine` tabs, `feed.xml`, and all SEO/JSON-LD output.

## 2. Information architecture

| Route | Role today | Role after |
|---|---|---|
| `/compare` (no `?type`) | Type-card landing + popular strip | Discovery landing (mockup 1) |
| `/compare?type=x` | Picker workspace | Unchanged logic; `StepHeader` restyled into a category header |
| `/compare?mine=true` | My quick comparisons | Unchanged |
| `/compare/[category]` | — (would 404 via `[slug]`) | **New**: family browse page (mockup 2) |
| `/compare/[slug]` | Quick comparison result | Visual upgrade only |
| `/comparisons` | Flat published list | Trending row + facet rail + vs-cards |
| `/comparisons/[slug]` | Article layout | Matrix + item-card upgrade only |

### 2.1 Category routing

New route `apps/main/src/routes/(app)/compare/[category=compareCategory]/` with a SvelteKit param matcher at `apps/main/src/params/compareCategory.ts`.

- The matcher validates the segment against a shared const of **family slugs** (sync check, no API call): `location`, `education`, `visa-routes`, `banking`, `embassies`, `cost-of-living`, `travel`, `jobs`. The const lives in `apps/main/src/lib/components/compare/compareFamilies.ts` (shared with the landing card builder) and mirrors the `family` keys already emitted by `type.config.family` / `FAMILY_ORDER` in `TypeCardGrid.svelte`.
- SvelteKit sorts `[category=matcher]` before plain `[slug]`, so `/compare/location` → browse page and `/compare/some-comparison-slug` → quick result, unchanged.
- **Known trade-off (accepted):** a quick comparison literally slugged `location` would be shadowed. Slugs are generated, so this is effectively impossible; it is documented in a code comment.
- A family slug not in the const falls through to `[slug]` and renders the quick-result 404, same as today. Adding a new family later = one-line addition to the const.

### 2.2 Family ↔ type mapping

Families come from `type.config.family` (already used by `TypeCardGrid`). The category page loads all types, groups by family, and the family page browses comparisons across its member types.

- Family with several types (e.g. `location` → countries/states/cities): the refine rail lists each type as a subcategory filter.
- Family with one type: rail still renders, with a single subcategory.
- Ungrouped types (no `family`) never produce a category page; their landing card links to `/comparisons?filter_type=<slug>` instead.

## 3. Backend changes (additive, `internal/comparison`)

### 3.1 `preview_items` on list summaries

`GET /jce/comparisons` currently returns `gen.ListComparisonsWithFiltersRow` rows directly. Wrap each row in a `JCEComparisonListItem` DTO:

```json
{
  "...existing summary fields...": "...",
  "preview_items": [
    { "title": "Canada", "image_url": "...", "geo_code": "ca", "country_code": "CA", "item_type": "country" }
  ]
}
```

- `preview_items` = first ≤3 `comparison_items` (by `position`, `deleted_at IS NULL`) per content row, hydrated in **one batched query** across the page's content IDs (`ListComparisonItemPreviews(content_ids)`), then image_id → media URL and geo_space_id → geo/country code resolved in batch through the existing resolver/media paths. **No N+1.**
- Hydration is best-effort: a failed resolve drops that preview field, never the list item.
- Applied to `ListJCEComparisons` only (public directory). The admin list and `mine` list can adopt it later if needed — not required for this spec.

Frontend type addition in `apps/main/src/lib/types/jce.ts`:

```ts
export interface JCEPreviewItem {
	title: string;
	image_url?: string;
	geo_code?: string;
	country_code?: string;
	item_type?: string;
}
// JCEComparisonSummary gains: preview_items?: JCEPreviewItem[];
```

### 3.2 `filter_type` accepts CSV

`ListJCEComparisons` handler resolves `filter_type` (and legacy `type`) via `GetComparisonTypeBySlug` → single `TypeID`. Change to:

- Split `filter_type` on `,`, trim, resolve each slug. Unknown slug → keep existing `type_not_found` 404.
- `JCEComparisonFilter.TypeID pgtype.UUID` → `TypeIDs []pgtype.UUID`.
- SQL (`internal/db/queries/comparison.sql`): `ListComparisonsWithFilters` + `CountComparisonsWithFilters` type predicate becomes `ci.type_id = ANY(sqlc.narg(type_ids)::uuid[])` with NULL/empty-array = no filter. Regenerate sqlc.
- Single-slug calls (all existing callers) behave identically.

### 3.3 Type facet counts

New opt-in param `facet=types` on `GET /jce/comparisons`. When present, the handler runs a new `CountComparisonsByType` query — same WHERE clause **minus the `filter_type` item predicate** — grouped by the comparison's primary type (first item's `type_id`, same rule the list uses for `type_name`) and returns:

```json
"meta": { "facets": { "types": [{ "type_id": "...", "slug": "countries", "count": 16 }] } }
```

Optional companion param `facet_scope=<csv-of-type-slugs>` restricts facet counting to that type set. This matters on family pages: `/compare/location?sub=countries` filters items to `countries` but the rail must still show counts for all member types (`countries=16, states=5, cities=3`), so the page sends `facet_scope=countries,states,cities`. `/comparisons` sends `facet=types` alone, yielding the all-types distribution for switching.

Slug/name enrichment reuses the type-definition lookup already used for `type_name`. `facet` absent → no facet query runs (hot paths like `mine` stay cheap).

### 3.4 No new sorts needed

`sort` already supports `popularity` and `created_at` — these map directly to the mockup's **Popular** and **Recently added** pills.

## 4. Shared components (new)

All under `apps/main/src/lib/components/compare/`:

- **`compareFamilies.ts`** — `COMPARE_FAMILIES` const (slug → i18n key, icon, tint), family grouping helper (types → family buckets + ungrouped), shared by landing, matcher, category page.
- **`CompareCategoryCard.svelte`** — tinted icon tile, name, subtitle (member type names, else `item_ref_types` labels), comparison count, chevron → href.
- **`ComparisonVsCard.svelte`** — the mockup's signature card: split-image header (two `preview_items` images, diagonal split, fallback tinted placeholder), flag-pair "vs" medallion when `geo_code`/`country_code` present (existing `flagUrl()`/`flagEmoji()`), badge chip, title, excerpt, "Compare now →", bookmark icon + formatted view count. Props: `summary`, `badge?`.
- **`CompareBadge.svelte`** — Popular/Trending/Hot chip. Derivation (v1, client-side, documented): position ≤2 in `popularity` sort → *Popular*; `created_at` within 14 days → *Trending*; `view_count` ≥ a configurable threshold → *Hot*. One badge max, priority Popular > Hot > Trending.
- **`CompareRefineRail.svelte`** — left rail: search input, category list with facet counts, optional themes section. Themes render **only** when a real facet exists (see §7) — never hardcoded.
- **`MatrixHighlight`** helpers in `lib/utils/comparison.ts` — per-row best-value detection for `rating`/`number`/`currency` dimension types (higher = better, except dimensions flagged `lower_is_better` in config). Returns winning cell IDs for green-tint rendering.
- **`HeroArtCompare.svelte`** — upgraded: three stacked entity cards (header band + two dot-metric rows each) over the existing orbit/"VS" node, evoking the mockup's Canada/Australia/Germany cards without photos. Stays SVG + `accent`-themeable.

## 5. Page changes

### 5.1 `/compare` landing

- `EntityHero` retained (ADR-058 shell consistency) with new art; description copy updated.
- New **"What would you like to compare?"** panel: search input (Enter → `/comparisons?q=`), category `<select>` (families + ungrouped types, → respective browse URLs), Popular/Recently-added pill toggle bound to the trending strip's sort.
- **Category grid**: `CompareCategoryCard`s — families first (in `COMPARE_FAMILIES` order), then ungrouped types.
- **Trending strip**: `ComparisonVsCard`s fed by `/jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&sort=<popularity|created_at>` — replaces today's plain popular cards.
- **"Need help comparing?"** card: opens existing `AiQuickPanel` (feature-gated exactly as today); for guests it links to `/login`.
- `?type=` and `?mine=true` flows below are untouched except `StepHeader` gaining the category-header style (icon tile + breadcrumb + count chips) — same props, same callbacks.

### 5.2 `/compare/[category]` browse page

- `+page.server.ts`: load types, resolve family from the matched param (404 if family unknown — unreachable in practice since the matcher pre-filters, but kept as a guard), fetch `/jce/comparisons?filter_type=<member slugs CSV, or single sub slug>&filter_depth=detailed&filter_status=published&filter_visibility=public&sort=&q=&page=&facet=types&facet_scope=<member slugs CSV>`. Subcategory selection rides the `?sub=<slug>` URL param.
- Layout: breadcrumb Home › Compare › `<Family>`; icon-tile hero + description + stat chips (`{N} comparisons`, `Updated weekly` i18n strings).
- "Can't decide?" card → **Start custom comparison** → `/compare?type=<slug>`; when the family has >1 member type it opens a small chooser listing the types.
- `CompareRefineRail`: search box (`q`), subcategory list with `facet.types` counts (All / each member type → re-queries with single `filter_type`), themes section only if a dimension/tag facet ships (§7 — out of scope v1, section hidden).
- Grid of `ComparisonVsCard`s + sort dropdown (`meta.sort_options`) + existing `PaginationControls`.
- Bottom CTA strip: preview-item avatars + "Start custom comparison".
- SEO: `SeoHead` with family name/description, canonical `/compare/<family>`, breadcrumb JSON-LD. Indexable (it's a directory page).

### 5.3 `/comparisons` index

- Trending row (`ComparisonVsCard`, top 3 by popularity) rendered above the list when no filters/search/mine are active.
- Desktop: `CompareRefineRail` with the type facet counts replaces the filters modal; mobile keeps the existing `Modal`.
- List cards → `ComparisonVsCard` (same component; excerpt/counts already available).
- `mine` tab, pagination, guide slide-over, SEO tags unchanged.

### 5.4 Detail pages

- `/compare/[slug]` and `/comparisons/[slug]`: item cards render `image_url` (or flag when geo item) — the fields exist on `ResolvedItem` and are currently unused.
- Matrix: `MatrixHighlight` marks the best cell per comparable row (green tint + check icon); sticky first column on horizontal overflow.
- Everything else (ReactionRail, discussions, CreatorCard, recommendations, freshness) untouched.

## 6. What is explicitly NOT changing

- All picker state, URL param contracts (`type`, `source`, `q`, `sort`, `page`, `page_size`, scope params, `preselect`, `compare_token`, `fields`, `nested_type`, `mine`), and navigation helpers in `/compare/+page.svelte`.
- `createQuickComparison`, `createGuestSelection`, `promoteComparison`, `deleteComparison`, `from-token` flows.
- `feed.xml`, sitemap entries, existing SEO head output on all four routes.
- Admin/control comparison screens.
- `TypeCardGrid` stays in use for the picker's type re-selection path; the landing uses the new `CompareCategoryCard`. If the landing grid fully replaces picker entry, `TypeCardGrid` remains the fallback for ungrouped-only configurations.

## 7. Deferred / out of scope

- **"Popular themes" checkbox facets** — needs a dimension/tag facet on the API; rail section hidden until a real facet exists.
- **Editorial images for comparisons without preview items** — cards fall back to tinted placeholder + flag medallion/initials.
- **Bookmark action on cards** — bookmark button renders only when viewer-state bookmarks cover comparison content; otherwise the icon is omitted (view count always shown).
- **Backend-driven badge rules** — v1 derives client-side per §4.
- New families added by admins require a one-line addition to `COMPARE_FAMILIES` (same limitation as today's `FAMILY_ORDER`).

## 8. i18n, a11y, theming

- All copy through `t()` with `jce.compare.*` / `jce.comparisons.*` keys; no hardcoded strings (Rule 16).
- Cards keyboard-focusable with visible focus rings; `aria-pressed` on badge pills where they act as toggles; flag medallions `aria-hidden` (titles carry the names).
- Colors strictly via `--color-*` tokens and per-family tints defined in `COMPARE_FAMILIES`; dark mode inherits from tokens.
- `prefers-reduced-motion` respected on any card hover/reveal transitions.
- RTL: chevron/arrow icons keep the existing `rtl:-scale-x-100` convention.

## 9. Testing

- `compareFlow.test.ts` siblings: unit tests for family grouping helper, badge derivation, matrix best-value detection.
- Backend: `filter_type` CSV resolution (single, multi, unknown slug → 404), `preview_items` batching (assert single items query per page), facet counts shape.
- Route-level: matcher unit test or e2e assertion that `/compare/<family>` renders the browse page and `/compare/<unknown>` still hits `[slug]`.
- Verify `bun.lock` stays at root only; no new deps expected.

## 10. Delivery order

1. Backend: `preview_items` DTO + `filter_type` CSV + `facet=types` (+ sqlc regen).
2. Shared components (`compareFamilies`, `ComparisonVsCard`, `CompareCategoryCard`, `CompareRefineRail`, `CompareBadge`, matrix helpers, `HeroArtCompare`).
3. `/compare` landing.
4. `/compare/[category]` + param matcher.
5. `/comparisons` index + detail-page matrix/item upgrades.
