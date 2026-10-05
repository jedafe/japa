# Wiki Attractions — Design

**Date:** 2026-10-05
**Spec home:** `.jeos/sections/80-destination-wiki.md` (Amendment — Wiki-Only Space Types & Attractions)
**Status:** Design

## Overview

Attractions — built sights (museums, landmarks, monuments) and natural wonders
(waterfalls, mountains, rock formations, lakes, caves, parks) — become a new
**wiki-only space type** `attraction`. An attraction is a Space that lives
exclusively in the Destination Wiki: it gets the full wiki surface (guide index,
category pages, fields, revisions, community edits, breadcrumbs, rollups) but
none of the community surface (feed, join, posting, members).

Parent types (city, state, country) gain an **Attractions** directory-mode
category whose page renders an image-first card grid of linked attraction
spaces — the "attractions in {place}" rollup the feature is built around.

The design deliberately reuses the §80 machinery end-to-end: a wiki type is a
`space_types` row + `wiki_type_configs` + `wiki_type_parents` edges; directory
rollups already walk `space_parent_spaces ∪ parent_id` transitively; spaces
already carry `logo_media_id` / `header_image_id` / `description` /
`wiki_content`; and `space:auto_populate` already seeds Wikipedia/Wikidata data.
New work is concentrated in four places: the `wiki_only` gate, rich directory
cards, gallery (`image_multi`) rendering, and the bulk seeder.

## Goals / Non-Goals

**Goals**

- `attraction` space type, wiki-only, child of city/state/country wiki types.
- Attraction page = its wiki index: title, summary/about, cover image, logo,
  gallery, structured quick facts, and guide categories.
- Parent pages gain an "Attractions" directory category with rich cards
  (cover image + blurb), type filter, scoped search — transitive, so a country
  page surfaces its cities' attractions.
- Admin creation + Wikidata bulk seeding; Wikipedia/Wikidata auto-enrichment
  per attraction via the existing `space:auto_populate` task.
- Keep the pattern reusable for sibling place types later (restaurant, hotel).

**Non-goals (v1)**

- Community-submitted attractions (phase 2 — the field-ops review queue can
  absorb it later; space creation stays admin/seeder in v1).
- Reviews/ratings/booking integrations on attraction pages.
- A generic "places" type — the schema stays open but we ship `attraction`.
- Map rendering of coordinates (the `coordinates` field stores and displays;
  a map widget is a later enhancement).

## 1. Data Model

### `space_types.wiki_only` (new column)

```sql
ALTER TABLE space_types ADD COLUMN wiki_only boolean NOT NULL DEFAULT false;
```

The capability flag that suppresses the community surface for a whole type.
Exposed on the space DTO as `space_type.wiki_only` (and in the space-types
admin DTO). Reusable — any future type (`restaurant`, `hotel`, `landmark`)
flips the same flag.

### Seed rows

- `space_types`: `attraction` — name "Attraction", icon `landmark`,
  `category 'geo'` (same grouping as country/state/city), `wiki_only = true`.
- `wiki_type_configs`: enabled, `label 'Attractions'`, `seed_stubs = true`,
  icon `landmark`.
- `wiki_type_parents`: `attraction` → `city`, `state`, `country`. Parent
  attachment uses the existing `space_parent_spaces` junction +
  `spaces.parent_id`: primary parent is the nearest container (city preferred;
  state/country direct for rural attractions — a national park may have no
  city). Border attractions (Victoria Falls → Livingstone *and* Victoria Falls
  Town) link multiple parents; the "also under X" alternates UI handles it.

### Attraction category template (seeded per type via existing admin API)

| Category slug | Label | Bound group `wiki_attraction_*` | Notes |
|---|---|---|---|
| `overview` | Overview | kind (select), address (text), coordinates (text `lat,lng`), opening_hours (textarea), entry_fee (price_range), website (url), wikidata_id (text), **gallery (image_multi)** | Stub page — the profile. Prose = About. |
| `plan-your-visit` | Plan Your Visit | getting_there (textarea), best_time (text), tips (textarea) | Optional prose page. |

The `kind` select seeds: `museum`, `landmark`, `monument`, `historic-site`,
`religious-site`, `market`, `waterfall`, `mountain`, `rock-formation`, `lake`,
`cave`, `beach`, `forest-reserve`, `canyon`, `hot-spring`, `national-park`,
`garden`, `zoo`, `theme-park`, `other`. Admin-extensible — it's field options,
not an enum column.

### Parent templates gain the Attractions directory category

`attractions` category on the `city`, `state`, `country` type templates:

```json
rollup: { "enabled": true, "mode": "directory", "levels": ["attraction"] }
```

`levels` pins the listing to attraction children only — a city's directory
doesn't absorb universities. The existing `empty` flag hides the tab/card on
spaces with no attractions.

## 2. Wiki-Only Gating

**Backend (enforcement):**

- `CreateSpace`/`UpdateSpace` for a `wiki_only` type force the axes to
  `visibility=public`, `posting/replying/commenting=admins`,
  `joining_perm=invite` regardless of request values — a wiki-only space can
  never drift into a community.
- Content creation (`POST /contents` into the space), `POST /spaces/{slug}/join`,
  and `POST /spaces/{slug}/rules` reject with `409 wiki_only_space` when the
  target space's type is wiki-only. Follow/save stays allowed — it's a
  bookmark, not a community surface.
- The type flag is read via a single `space_types` join on the enforcement
  paths (already loaded in most of them).

**Frontend (presentation):**

- `/spaces/[slug]` on a wiki-only type redirects to `/wiki/[slug]` — the wiki
  index IS the attraction page. The space page's community shell (feed
  composer, join button, members tab) never renders for these types.
- Wiki index hides the "community help" CTA (`/spaces/…` link) for wiki-only
  spaces; keeps Save + Share.

## 3. Rich Directory Cards

`ListWikiDirectoryChildren` gains `logo_media_id`, `header_image_id`,
`description`, and `wiki_content->>'extract'` (truncated). `WikiDirectoryChildDTO`
gains:

```go
ImageURL string `json:"image_url,omitempty"` // header preferred, logo fallback — imgproxy 640×360 webp
Blurb    string `json:"blurb,omitempty"`    // description, else ~140-char wiki extract
Kind     string `json:"kind,omitempty"`     // first value of the child's `kind` cfv, for the card chip
```

Resolution uses the wiki service's already-wired `mediaURLResolver` — same
pattern as `resolveLogoURL`, larger crop for covers. All enrichment is
additive and best-effort: a child with no image falls back to today's text
card, so existing directories (states, universities) are unchanged visually
until children carry images.

`WikiDirectoryPanel.svelte` renders an image-first card when `image_url` is
present (cover top, name, kind chip, blurb line-clamp-2) and groups/searches
exactly as today.

## 4. Gallery

`image_multi` values store media IDs in `custom_field_values.value_json`
(`{"media_ids": [uuid, ...]}`). Today the entity-fields read path passes
`value_json` through verbatim and `EntityFieldsPanel` formats it as text —
useless for images. Changes:

- **Read path**: `image_single`/`image_multi`/`media` field values resolve
  media IDs → `{id, url, alt}` objects in `EntityFieldGroupDTO` field values
  via `s.media` (imgproxy, card-size crop + full-size for lightbox).
- **Render**: `EntityFieldsPanel` renders `image_multi` values as a thumbnail
  grid; `image_single` inline. A `gallery`-keyed field renders as a full-width
  strip on the attraction index/overview page. Lightbox is a follow-up — v1
  thumbnails link to the full media URL.
- **Upload/edit**: community field ops already accept `set_value` on image
  fields; the SuggestEdit UI gains a media picker bound to the existing media
  upload endpoint (same `MediaLibraryModal` flow the space logo picker uses).

## 5. Creation & Seeding

**Admin manual creation** — no new machinery: `POST /spaces`
(space_type_id=attraction, name, description, parent links via
`POST /spaces/{slug}/parents`), then `space:auto_populate` enriches. Admin UI
adds "Attraction" to the space-type picker and a parent-space autocomplete —
both exist as components.

**Auto-populate** — `AutoPopulateSpace` gains `case "attraction"`:

- Wikipedia summary → `wiki_content` (extract = About fallback) + original
  image → `header_image_id`.
- Wikidata `wbsearchentities` scored on attraction keywords — existing
  `geoTypeKeywords` mechanism gains an `attraction` entry:
  `{attraction, museum, landmark, monument, memorial, park, garden, castle,
  waterfall, mountain, rock, monolith, inselberg, lake, cave, falls, zoo}` —
  with the parent-space-name boost reused from geo scoring.
- P18 image → header (preferred), P154/P94 → logo, `official website` claim →
  `website` field, `P625` coordinate → `coordinates` field. Custom-field
  upserts go through the existing `CustomFieldsEnricher` interface.

**Bulk seed — new task `wiki:seed_attractions`** (§11 catalog, low tier):

Payload `{parent_space_id, limit?, dry_run?}`; triggered by
`POST /api/v1/admin/wiki/seed/attractions { within: "<space_slug>", limit?, dry_run? }`
(`wiki.manage`). Flow:

1. Resolve the parent space; run Wikidata SPARQL: entities `wdt:P31/wdt:P279*`
   one of the seeded attraction classes (`tourist attraction`, `museum`,
   `landmark`, `monument`, `waterfall`, `mountain`, `rock formation`,
   `monolith`, `lake`, `cave`, `national park`, `garden`, `zoo`,
   `amusement park`, `beach`, `historic site`, `religious site`)
   `wdt:P131*` located within the parent — OR carrying the parent's QID via
   `P131` at any hop (covers the recursive admin-entity chain).
2. Dedup on `wikidata_id` (stored in `wiki_content.wikidata_id` and the
   `wikidata_id` cfv) and on slug collision (suffix `-2`, `-3`…).
3. Create each `attraction` space linked to the parent
   (`space_parent_spaces` + `parent_id`); multi-`P131` entities link each
   resolved JapaConnect parent (border attractions).
4. Enqueue `space:auto_populate` per created space (idempotent — the flag on
   `TypeSpaceAutoPopulate` already collapses repeat enqueues).
5. Result row `{created, skipped_duplicate, failed, slugs[]}` → audit log;
   `dry_run` returns the candidate list without writes.

External API pacing reuses `wikiRateLimiter` (`spaces.wiki_rate_interval_ms`).

## 6. API / DTO surface changes

| Route | Change |
|---|---|
| `GET /api/v1/wiki/:space/:page` | `directory.children[]` gains `image_url`, `blurb`, `kind` |
| `GET /api/v1/wiki/entities/:type/:id/fields` | `image_*`/`media` field values return `{id,url,alt}[]` |
| `GET /api/v1/spaces/:slug` (+ list payloads) | `space_type` object gains `wiki_only` |
| `POST /api/v1/admin/wiki/seed/attractions` | **New** — enqueue `wiki:seed_attractions` (`wiki.manage`) |
| `POST /api/v1/spaces`, `PATCH` | wiki-only types force restrictive axes (silent override → 200 with normalized axes) |
| `POST /api/v1/contents`, `/spaces/:slug/join`, `/spaces/:slug/rules` | `409 wiki_only_space` on wiki-only spaces |

No new public routes. RBAC reuses `wiki.manage` — no new permission nodes.
Feature flags: none new — `wiki.enabled` governs.

## 7. Frontend (apps/main)

- `WikiDirectoryPanel`: image-first cards when `image_url` present.
- `EntityFieldsPanel` / field renderers: `image_multi` thumbnail grid +
  `image_single` inline image; media picker in `SuggestEdit` for image fields.
- Wiki index (`/wiki/[space_slug]`): gallery strip under the hero when the
  overview group's `gallery` field has values; hide community CTA for
  wiki-only types; `kind` chip next to the title.
- `/spaces/[slug]` redirect for wiki-only types.
- Admin (`apps/admin`): attraction in space-type pickers; parent autocomplete;
  "Seed attractions" action on the wiki type detail (calls the seed endpoint,
  dry-run preview first).

## 8. Testing

- Store: directory children projection (images/blurb/kind present; transitive
  depth respected; wiki-only types excluded when not enabled).
- Service: `wiki_only` perm-axis coercion on create/update; `409` on
  post/join/rules into attraction spaces; directory `empty` flag on spaces
  with no attractions.
- Seeder: dedup on wikidata_id + slug; multi-P131 multi-parent linking;
  `dry_run` writes nothing; rate-limited retries reuse the geo path.
- Frontend: card fallback (no image → text card), gallery grid render,
  spaces→wiki redirect.

## 9. Implementation phases

1. **Migration + JEOS**: `space_types.wiki_only`, attraction seed rows,
   `wiki:seed_attractions` in §11, §80 amendment (this spec), routes catalog.
2. **Backend gating + DTO**: wiki_only enforcement, space DTO flag,
   directory-card enrichment.
3. **Seeding**: `populateAttractionSpace`, `wiki:seed_attractions` task +
   admin trigger route.
4. **Frontend**: rich cards, gallery rendering, spaces redirect, admin seed UI.
