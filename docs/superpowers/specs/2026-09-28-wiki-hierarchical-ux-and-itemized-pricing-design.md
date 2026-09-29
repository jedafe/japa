# Wiki Hierarchical UX & Itemized Pricing — Descendant Rollups, Page Templates, and Item Tables

**Date:** 2026-09-28
**Status:** Draft
**Builds on:** §06 (Universal Custom Fields), §80 (Destination Wiki + Community Field Ops), §30 (JCE), 2026-09-26 wiki-types-and-field-group-pages design, migration 000739
**Scope:** `internal/wiki`, `internal/customfields`, `internal/comparison`, `internal/spaces`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§80 amendment, §06, §30, catalogs)

## 1. Problem

The 2026-09-26 design delivered typed wiki templates: a wiki type owns a
category template, each category binds a dedicated per-level field group,
and every page renders its category's group schema + the host space's cfv
values. Three gaps remain before the wiki can render the target UX:

- **No itemized data.** A field group is a flat `key → value` list. The
  target UX (e.g. a "Prices in {City}" page) renders *item tables* — rows
  of `item | average | range` grouped into sections (Food & Groceries,
  Transportation, Utilities). There is no repeating/row concept, no
  multi-part value (avg + min + max), and no sub-grouping inside a group.
- **No hierarchical reading.** Breadcrumbs stop at
  `Wiki › {Type} Guides › {space} › {page}`; the space's ancestor chain
  (Toronto → Ontario → Canada) is not resolved into the response. Pages
  have no sibling ordering — no Previous/Next, no "See more prices →"
  navigation between sibling pages of a category.
- **No descendant aggregation.** A country's Cost of Living page must
  show min/max/avg over its descendant cities' cfv values, plus a
  per-city table; a city's page must show a per-neighborhood table. The
  page response today carries only the host space's own values.
- **`wiki_type_parents` is write-only.** The admin declares allowed
  nesting (`state→country`, `city→state`, `university→{all geo}`) but
  nothing reads the edges at runtime — the geo chooser on `/wiki` is a
  hardcoded `GEO_WIKI_TYPES` set, and no surface asks "which types nest
  under this type?" The edges should drive navigation, breadcrumbs, and
  rollup scoping.
- **Page schema is category-level only.** Every page in a category
  shares the category's bound group. A sibling page like "Prices in
  {City}" needs its own (type-level, shared-across-spaces) schema —
  `wiki_pages` has no `field_group_id`.

## 2. Goals

1. **Item tables inside a page.** One bound group renders as multiple
   sectioned item tables (Food & Groceries, Transportation, …), each row
   carrying `average`, `range_lo`, `range_hi`, `unit`. Community ops edit
   individual items; new items are proposed through the same ops queue.
2. **Shared schema across spaces.** The "prices" schema for cities is
   *one* group bound by a type-level template — every city's Prices page
   shares the same field keys so JCE can compare city↔city on any item.
3. **Ancestor-chain breadcrumbs.** Page responses carry the space's
   resolved ancestor chain so the frontend renders
   `Wiki › Country Guides › Canada › Cities › Toronto › Cost of Living › Prices in Toronto`.
4. **Type edges drive the hierarchy UX.** `wiki_type_parents` becomes
   load-bearing: it scopes which descendant types a rollup may read,
   which parent chain a multi-parent space's breadcrumb prefers, and
   how the landing chooser drills down (replacing the hardcoded geo
   set). Space-level links (`spaces.parent_id` / `space_parent_spaces`)
   remain the only source of *actual* parentage — the edges declare
   which types are allowed/expected in the chain.
5. **Descendant rollups.** A category may opt into descendant
   aggregation: aggregate stats (min/avg/max per field key) plus a
   children table (descendant spaces + their values) power "Average
   Monthly Expenses" cards and "Cost of Living by City/Neighborhood"
   tables.
6. **Sibling navigation.** Pages order within a category (`position`);
   the response carries siblings for Prev/Next and "View more →" links.
7. **Zero divergence from §80 contract.** `custom_field_values` stays the
   single store of record; all new surfaces merge cfv at read; writes
   flow through the ops queue.

## 3. Information architecture (target surfaces)

```
/wiki                                landing (geo chooser + per-type sections) — exists
/wiki/{country}                      space index — category cards — exists
/wiki/{country}/{category}           category page — prose + aggregate cards
                                     + descendants table + own fields
/wiki/{city}                         space index — exists
/wiki/{city}/{category}              category page — prose + own field values
                                     + child-level descendants table (neighborhoods)
/wiki/{city}/{page}                  template page ("Prices in {City}") —
                                     sectioned item tables from its bound group
```

Breadcrumb rule everywhere:
`Wiki › {type} Guides › {ancestor spaces…} › {space} › {category} › {page}`.

## 4. Design — Page templates & page-level field groups

Categories are per-type template rows; sibling *pages* under a category
need the same treatment or every space's "Prices" page would bind an
ad-hoc group and comparability dies. Add a second template layer:

```sql
CREATE TABLE wiki_type_page_templates (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  category_id    uuid NOT NULL REFERENCES wiki_type_categories(id) ON DELETE CASCADE,
  slug           text NOT NULL,              -- 'prices' → /wiki/{space}/prices
  label          text NOT NULL,              -- 'Prices in {name}' (token {name})
  field_group_id uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
  position       int NOT NULL DEFAULT 0,
  is_system      boolean NOT NULL DEFAULT false,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (category_id, slug)
);
```

- Template create auto-provisions `wiki_{type}_page_{slug}` (same
  get-or-create pattern as `provisionCategoryGroup`) plus the
  `space_type:{slug}` assignment — **one group shared by every space of
  the type**, which is what makes city↔city item comparison
  representable at all.
- `wiki_pages` gains `field_group_id` + `page_template_id`, both
  nullable. Page creation resolves the template by `(category, slug)`:
  the row stores the resolved binding so the group survives template
  deletion, and `FieldGroupID` in the page DTO is
  `page.field_group_id ?? category.field_group_id`.

```sql
ALTER TABLE wiki_pages
  ADD COLUMN page_template_id uuid REFERENCES wiki_type_page_templates(id) ON DELETE SET NULL,
  ADD COLUMN field_group_id   uuid REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
  ADD COLUMN position         int  NOT NULL DEFAULT 0;
```

- A page with no template match (ad-hoc slug) behaves as today: it
  renders the category's group. Template-bound slugs are reserved per
  type — creating `/wiki/{city}/prices` always lands on the template.
- `seed_stubs` stays category-level; template pages are lazy like
  category pages (rendered empty until first prose/ops submission).

### Why not entity-scoped group assignments
`custom_field_group_assignments.entity_id` could bind a group to
`entity_class='wiki_page', entity_id=<page>` — but that scoping is
*per-instance*: every city's page would get its own group row, breaking
shared field keys. Type-level templates keep schema shared by
construction; assignment scoping remains the escape hatch for true
one-off overrides.

## 5. Design — Itemized price fields

Item tables decompose into two additions to the field system:

```sql
ALTER TABLE custom_fields ADD COLUMN section text;  -- 'food_groceries' | 'transportation' | …
```

- **`section` groups fields within a group.** The page response renders
  `fields.sections[]` ordered by section position; each section is one
  item table (and one sub-tab / one TOC anchor / one "View more" filter).
  `section` is a free token constrained to `^[a-z0-9_]+$`; the group's
  description or a small `wiki_group_sections` seed holds section labels
  (decision: keep labels in a `custom_field_groups.meta` JSONB or a
  side table — pick at implementation; not a new field type).
- **Composite price value.** A new proposable field type `price_range`
  stores `{avg, min, max}` (+ `unit`, `currency` inherited from group
  meta or field options) in `value_json`. One field = one item row
  (`milk_1l` → `{avg: 2.29, min: 1.79, max: 2.99}`). `set_value` ops
  already accept JSON payloads; coercion adds a `price_range` branch.
  JCE dims project from fields → `milk_1l` becomes a comparable
  dimension automatically, typed `currency`-like on `avg`.
- **Rejected alternatives:** (a) three namespaced fields per item —
  ~240 fields/group, no atomic edit, community can't propose items;
  (b) one JSON field holding the whole table — kills per-field
  permissions, per-item ops, and dimension projection; (c) a
  `price_item` entity class — works via `data_entity_type` but
  reintroduces per-space instance rows and an item-dictionary governance
  problem the shared-group approach avoids.

## 6. Design — Ancestor chain & breadcrumbs

Page and space-detail responses gain `ancestors`:

```json
"ancestors": [
  { "slug": "canada",  "name": "Canada",  "level": "country", "type_label": "Country Guides" },
  { "slug": "ontario", "name": "Ontario", "level": "state",   "type_label": "Cities" }
]
```

- Resolution reuses the `within` join family: follow
  `space_parent_spaces` ∪ `spaces.parent_id` upward to depth 3
  (country→state→city).
- **Type edges arbitrate multi-parent chains.** When a space has
  several parents (university under city *and* country), the primary
  chain follows `spaces.parent_id`; among `space_parent_spaces`
  parents, the preferred ancestor is the one whose `space_type` is a
  *declared* parent in `wiki_type_parents` — e.g. a university under a
  city and a country renders `… › Cities › Toronto › {uni}` because
  `university→city` is declared. Undeclared parents are excluded from
  the chain but may surface in `ancestors_alt` as "also under X".
- **Intermediate level labels come from the edges.** The "Cities"
  segment between `Canada` and `Toronto` exists because `city`'s
  declared parent chain passes through `state` — the label is the child
  type's plural label (`wiki_type_configs.label`), not a hardcoded
  string. A type with no declared parent renders no intermediate label.
- `type_label` resolves through the ancestor's wiki type label —
  breadcrumb segments become links to `/wiki/{slug}`.
- Frontend renders the full chain; `leaf` space links remain
  `/wiki/{slug}` (no route changes).

## 7. Design — Descendant rollups

`wiki_type_categories` gains an opt-in aggregation config:

```sql
ALTER TABLE wiki_type_categories ADD COLUMN rollup jsonb;
-- {"enabled": true, "metrics": ["min","avg","max"], "levels": ["city"], "section_limit": 8}
-- `levels` optional: when omitted, the descendant type set is derived
-- from wiki_type_parents (all types declared under the space's type).
```

When `rollup.enabled`, `GET /wiki/{space}/{page}` (and the lazy-template
page) carries:

```json
"rollup": {
  "fields":    [ { "key":"rent_1br", "label":"Accommodation",
                   "min": 700, "avg": 1450, "max": 2000, "n": 12 } ],
  "children":  [ { "slug":"toronto", "name":"Toronto, ON",
                   "values": { "rent_1br": {"avg":2100}, "food": {"avg":450} } } ]
}
```

- **Descendant resolution** reuses the `within` machinery
  (`space_parent_spaces` ∪ `parent_id`, depth ≤3). The descendant *type
  set* defaults to the transitive closure of `wiki_type_parents` under
  the space's type (country → `{state, city, university, …}`); a
  category may narrow it with explicit `rollup.levels`. Enabling a new
  child type (e.g. `city → neighborhood`) automatically makes it
  eligible for the parent's rollup — no config edit — while actual
  membership still requires real space-level links: the edges scope,
  `space_parent_spaces`/`parent_id` supplies the rows.
- **Source fields** = the union of field keys from the descendant types'
  *same-category-token* groups (`cost_of_living` exists on city + state
  + country templates — the token is the cross-level join). Values read
  from `custom_field_values` at `entity_type='space'`, aggregated
  per key. Spaces lacking a value for a key are excluded from that key's
  `n`.
- **Cardinality guard:** `rollup` config caps `children` rows; the full
  set is always available through the existing
  `GET /wiki/destinations?within={slug}&level={level}` endpoint.
- The country/state category's own bound group stays **empty or
  national-only fields** (per prior decision: rollups are computed
  views, never stored — national-only data like visa fees are real
  fields on the country's group; the aggregate cards are not fields).

### Edge-driven landing chooser

`/wiki`'s drill-down stops hardcoding `GEO_WIKI_TYPES`: the chooser
levels come from `GET /wiki/types` extended with `parent_type_ids`
(currently admin-only on `GET /admin/wiki/types`). Picking a space
offers its declared *child types* — a country offers states and
universities; a state offers cities and universities — and each choice
issues `?parent={slug}&level={child_type_slug}` against
`/wiki/destinations`. Types with no declared parents are root chooser
options alongside `country`.

## 8. Design — Sibling ordering & page navigation

- `wiki_pages.position` (default 0; admins/community suggest order via
  ops `set_position` — new trivial op or admin-only PATCH).
- Page response gains `siblings`: `[{slug, title, position}]` — the
  space's active pages in the same category, ordered
  `position, created_at`. Frontend renders Prev/Next (with boundary wrap
  into the adjacent category's first/last page per template order) and
  "View more {section} prices →" links as
  `/wiki/{space}/{page}?section={section}` (section-filtered view of the
  same page — no new route).
- Category tab strip on the space index is unchanged (one tab per
  template category); sub-tabs *inside* a prices page are section
  anchors from `fields.sections`.

## 9. JCE comparability contract

- A `comparison_type_definition` binds `field_group_id =
  wiki_city_prices` (or any shared page-template group); dimensions
  project from fields as today. `price_range` fields map to
  `currency` dimension type on `avg` (`fieldTypeToDimensionType` gains a
  branch).
- **Section-scoped comparisons:** `comparison_type_definitions.config`
  gains `"sections": ["transportation"]` — dims filtered to that
  section's fields, yielding "Toronto vs Vancouver transportation"
  without a group per section.
- Level-purity is preserved: shared page-template groups are still
  per-type (`wiki_city_prices` ≠ `wiki_country_prices`), so JCE cannot
  construct cross-level comparisons — same invariant as §4 of the
  previous spec.
- Rollup surfaces never write back into cfv; JCE reads city rows
  directly.

## 10. API additions

```
-- Admin: page templates (under categories)
GET    /api/v1/admin/wiki/categories/:id/page-templates
POST   /api/v1/admin/wiki/categories/:id/page-templates   { slug, label, field_group_id?, position }
PATCH  /api/v1/admin/wiki/page-templates/:id              { label?, position?, field_group_id? }
DELETE /api/v1/admin/wiki/page-templates/:id              — blocked while bound pages carry prose/values

-- Admin: category rollup config rides the existing PATCH /admin/wiki/categories/:id
PATCH  /api/v1/admin/wiki/categories/:id                  { ..., rollup?: {...}|null }

-- Public: response additions only, no new routes
GET /api/v1/wiki/types                             — gains parent_type_ids
                                                     (same edges as the admin list; the
                                                     landing chooser is edge-driven)
GET /api/v1/wiki/:space_slug/:page_slug
    gains: ancestors[], rollup?, siblings[],
           fields.sections[] (section-grouped schema+values)
GET /api/v1/wiki/:space_slug
    gains: ancestors[] (space-level breadcrumb for the index page)
```

Field ops additions: `price_range` accepted by `propose_field`/
`upsert_field` (added to `ProposableFieldTypes`); `set_value` gains the
composite coercion. No new op names → no Section 11 changes.

## 11. RBAC & flags

- Page-template + rollup administration rides `wiki.manage`.
- `price_range` field editing rides `wiki.fields.*` +
  `custom_field_permissions` unchanged.
- Rollup computation is read-path only — no permission changes; cfv
  visibility still filtered by `ListVisibleFieldIDsForUser`.
- `wiki.enabled` master switch unchanged; a `wiki.rollup` flag is NOT
  added — the per-category `rollup.enabled` config is the switch.

## 12. Migration / rollout

1. `wiki_type_page_templates` + `wiki_pages` columns
   (`page_template_id`, `field_group_id`, `position`) +
   `wiki_type_categories.rollup` + `custom_fields.section`.
2. Register `price_range` in the field-type registry; `section` added to
   field admin UI and ops validation.
3. Seed city template: `prices` page template under `cost_of_living`
   with `wiki_city_prices` group + section seeds (food_groceries,
   restaurants, transportation, utilities, lifestyle); enable rollup on
   country/state `cost_of_living` categories
   (`{"enabled":true,"levels":["city"],"metrics":["min","avg","max"]}`).
4. Backend: ancestor resolver, rollup service, page response additions
   (fields.sections, siblings, rollup, ancestors).
5. Frontend (`apps/main`): breadcrumb chain, sectioned item tables +
   "View more" section filter, aggregate cards + children table +
   LayerChart donut, glance stat cards, Prev/Next, ON THIS PAGE TOC.
6. Admin (`apps/admin`): page-template CRUD under the existing
   Categories panel; rollup JSON editor on the category form; `section`
   on the field form.
7. JCE: `price_range`→currency mapping, `config.sections` dim filter,
   comparison type seed bound to `wiki_city_prices`.
8. Neighborhoods: `neighborhood` space type + wiki type
   (`city → neighborhood` parent edge) is a space-side seed — no wiki
   machinery changes; its rollup rides step 3's config.

## 13. Non-goals

- Storing aggregates in cfv — rollups are read-time views; a materialized
  cache is a later performance decision, not part of this spec.
- Cross-level field sharing — still forbidden; national fields live on
  the country's own group.
- Page templates for non-template slugs — ad-hoc pages keep the
  category group; forcing all pages through templates is a later call.
- Rewriting `data_entity_type` child-entity pages — they coexist; the
  neighborhood table uses rollup spaces, not `data_entity_type`.
- Front-end IA for non-geo types — university wikis inherit all of this
  but no university-specific mockups are in scope.

## 14. Open questions (decide at implementation)

- **Section label storage:** `custom_field_groups.meta` JSONB vs.
  `custom_field_sections` table vs. convention on field `key` prefix.
  Meta JSONB is the smallest change; a table gives admin CRUD for free.
- **Multi-parent ancestor display:** primary-chain-only vs. alternate
  parent badges on university pages.
- **Sibling wrap semantics:** strict category-only Prev/Next vs.
  flowing across categories in template order (mockups show both —
  needs a product call).
- **`prices` vs `cost_of_living` overlap:** the prices template group is
  item-level while `wiki_city_cost_of_living` is summary-level
  (accommodation_total etc.); keep both groups and let the summary
  fields be *entered* (not computed) in v1, or derive summary cards
  from price items later.
- **Unit/currency placement:** on the group meta (one currency per
  group's items) vs. per-field options — group meta is the mockup's
  model ("All prices in CAD").
