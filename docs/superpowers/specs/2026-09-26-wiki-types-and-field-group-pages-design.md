# Wiki Types & Field-Group Pages — Typed Wiki Templates over §06 Custom Fields

**Date:** 2026-09-26
**Status:** Design
**Builds on:** §06 (Universal Custom Fields), §80 (Destination Wiki + Community Field Ops + Tool Surface Hydration), 2026-09-24 wiki-community-fields design
**Scope:** `internal/wiki`, `internal/customfields`, `internal/spaces`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§80, §06, catalogs)

## 1. Problem

The Destination Wiki models each page as a TipTap document with a TEXT
`category` drawn from two hardcoded Go slices (`wikiCategories`,
`wikiUniversityCategories` in `internal/wiki/handlers.go`). Structured data
already rides alongside prose via Field Ops — but:

- **Page → schema binding is implicit.** A page's Data panel shows *every*
  field group assigned to the entity's class. There is no notion of "the
  `cost_of_living` page edits the Cost of Living group's fields."
- **Categories are code, not data.** Adding a category or a new wiki scope
  (a new entity kind with a wiki) requires a deploy.
- **Wiki types are implicit.** Geo (country→state→city) and university are
  the only two scopes, distinguished by `spaces.level`. The owner wants wiki
  types managed in admin like categories and pages: geo is the primary chain,
  and every other wiki type (university, and future types like embassy,
  neighborhood, school) is a child of one or all geo levels.
- **Downstream consumers need a contract.** JCE, guide-category structured
  data, and japa tools will source structured data from the wiki-maintained
  `custom_field_values` store (kept fresh by community + AI). The sourcing
  path must be uniform for every future consumer.

## 2. Goals

1. **Wiki types are managed rows, not code.** Admin CRUD for wiki types, each
   bound to a `space_type`, with declared parent types — geo levels are the
   primary chain; every other type nests under one or more of them.
2. **A wiki type owns a category template.** Each category is a row binding
   `slug + label + field_group_id`. `/wiki/{entity}/{category-slug}` renders
   the bound group's schema for every entity of that type — every country
   sees the same Overview fields, every city the same Cost of Living fields.
3. **Future wiki types are zero-code.** Create a space type → enable wiki →
   declare parents → bind category field groups. Navigation, categories,
   pages, and field editing all follow.
4. **One sourcing contract.** `custom_field_values` remains the single source
   of truth (§80 hydration amendment); JCE, guides, and tools merge cfv at
   read. Nothing reads wiki tables for data.

## 3. Design — Wiki Types

A wiki type is **not** a parallel type system: it is a `space_type` that has
a wiki enabled, plus wiki-specific configuration. Spaces already provide the
hierarchy (`spaces.parent_id` primary parent; `space_parent_spaces` M2M for
"child of any or all" — e.g. a university parented to both its city and its
country).

```sql
CREATE TABLE wiki_type_configs (
  space_type_id uuid PRIMARY KEY REFERENCES space_types(id),
  enabled        boolean NOT NULL DEFAULT true,
  label          text,          -- display override; defaults to space_types.name
  icon           text,
  seed_stubs     boolean NOT NULL DEFAULT false, -- auto-create stub pages for
                   -- new spaces of this type (the geo behavior today)
  position       int NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now()
);

-- Which wiki types a type may nest under. Geo chain models as:
--   state.parent → country; city.parent → state (and/or country).
--   university.parent → {country, state, city} ("any or all").
CREATE TABLE wiki_type_parents (
  space_type_id        uuid NOT NULL REFERENCES wiki_type_configs(space_type_id),
  parent_space_type_id uuid NOT NULL REFERENCES wiki_type_configs(space_type_id),
  PRIMARY KEY (space_type_id, parent_space_type_id)
);
```

`wiki_type_parents` declares the *allowed* nesting for navigation and admin
validation; actual per-instance parentage continues to live on
`spaces.parent_id` / `space_parent_spaces` — the wiki destinations endpoint
already filters children through both (`internal/wiki/store.go` `ListWikiDestinations`).

Seed rows: `country`, `state`, `city`, `university` configs; parents
`state→country`, `city→state`, `university→{country,state,city}`;
`seed_stubs=true` for `country` only (matches §80: geo stubs seeded,
university wikis start empty).

### Why not a standalone `wiki_types` table

Every wiki page is bound to a space; a wiki "type" is always "the type of
space it sits on." A separate registry would drift from `space_types` and
duplicate `space_types.field_group_id` (the existing per-type default group).
A config table over `space_types` keeps one type system.

## 4. Design — Category = Field Group (mandatory, per-level)

```sql
CREATE TABLE wiki_type_categories (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  space_type_id  uuid NOT NULL REFERENCES wiki_type_configs(space_type_id),
  slug           text NOT NULL,          -- page slug: 'overview','cost-of-living'
  category       text NOT NULL,          -- wiki_pages.category token: 'cost_of_living'
  label          text NOT NULL,
  field_group_id uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
  position       int NOT NULL DEFAULT 0,
  is_system      boolean NOT NULL DEFAULT false,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (space_type_id, slug),
  UNIQUE (space_type_id, category)
);
```

- Replaces the hardcoded `wikiCategories` / `wikiUniversityCategories` slices.
  `GET /wiki/categories?space={slug}` resolves space → `space_type_id` →
  rows (Rule 16: the frontend already consumes this endpoint).
- **`field_group_id` is mandatory** — the group IS the page's schema. A
  prose-only page is a category whose group has no fields. Every category
  owns a *dedicated per-level* group named `wiki_{type}_{category}`:
  `wiki_country_cost_of_living`, `wiki_state_cost_of_living`, and
  `wiki_city_cost_of_living` are three disjoint groups.
- Each group carries a `custom_field_group_assignments` row scoped to
  `entity_class = 'space_type:{slug}'`, so schema is type-level while
  `custom_field_values` stays entity-level. Adding a field to
  `wiki_city_cost_of_living` surfaces it on every city's page (each city
  stores its own value); Lagos article content never propagates to Kumasi.
- **JCE isolation by construction:** disjoint groups per level make
  city↔state or city↔country comparison unrepresentable — JCE only ever
  compares entities sharing the same group, hence the same field keys.
- Category create auto-provisions group + assignment when `field_group_id`
  is omitted (get-or-create by slug, so a group surviving a deleted
  category is reused). An explicit `field_group_id` rebinds an existing
  group — deliberate sharing only, not the default.
- `kind='mixed'` revisions already carry prose + field ops atomically.
- `space_types.field_group_id` remains the type's *default/catch-all* group
  (shown on the wiki index's Data panel); category groups are per-page.

### Page resolution

```
GET /wiki/{space_slug}/{page_slug}
  → space (incl. space_type_id, level)
  → wiki_pages row (prose + revision state; may not exist yet — lazy)
  → wiki_type_categories row for (space_type_id, page_slug)
  → bound group → fields (defs) + cfv values for (entity_type='space', space.id)
```

**Lazy pages:** a `wiki_pages` row is created on first prose submission. The
page renders "empty schema + no prose" from the category template before
then — eliminating the stub-seeding job for new categories/types; assigning
a category makes the page exist on every entity instantly.

**Why dedicated per-level groups (not one shared `cost_of_living` group
assigned to all three entity_classes):** a shared group would make every
field comparable across levels — JCE could then rank "Country A's cost of
living" against "City X's cost of living," which is a nonsense comparison.
Distinct groups keep the comparison surface level-pure by construction; the
cost of the choice is that a field wanted at multiple levels (e.g.
`rent_1br` on state AND city) must be added to each level's group.

## 5. Downstream sourcing contract

The wiki becomes the community+AI editing surface; `custom_field_values` is
the store of record (§80 Tool Surface Hydration already implements this for
tools and JCE). This spec adds the general rule:

- **Consumers merge cfv at read; they never read wiki tables.** JCE matrix
  cells hydrate via `dimension.source_field_id` → `field_values`; tool
  resolvers overlay cfv keys; guide-category structured data reads the
  entity's cfv via its `content_type:guide:{slug}` group.
- **Writes flow through the ops queue.** Community `set_value` ops and
  Content Agent/ingestion writes land in `custom_field_values` (AI rows carry
  provenance/`ai_exempt` handling per existing convention). Approved edits
  enqueue `wiki:fields_propagate` → JCE cache bust.
- **New consumers register a merge row** in the §80 merge-contract table
  (entity type → merge target), matching `visa_route`, `col_city`, etc.

## 6. API additions

```
-- Wiki types (admin)
GET    /api/v1/admin/wiki/types                        — list types + parent edges
POST   /api/v1/admin/wiki/types                        — { space_type_id, label?, parent_type_ids[], seed_stubs }
PATCH  /api/v1/admin/wiki/types/:space_type_id
DELETE /api/v1/admin/wiki/types/:space_type_id         — blocked while spaces of the type have wiki pages

-- Category templates (admin)
GET    /api/v1/admin/wiki/types/:space_type_id/categories
POST   /api/v1/admin/wiki/types/:space_type_id/categories   — { slug, label, field_group_id?, position }
PATCH  /api/v1/admin/wiki/categories/:id
DELETE /api/v1/admin/wiki/categories/:id                    — blocked while pages of that category have prose

-- Public
GET    /api/v1/wiki/types                              — enabled types (for nav/destinations level filter)
GET    /api/v1/wiki/categories?space=                  — now DB-driven; response shape unchanged
GET    /api/v1/wiki/:space_slug/:page_slug             — response gains `field_group` block
                                                        (schema + values) when the category binds one
```

`GET /wiki/entities/:type/:id/fields` is unchanged — it returns all groups;
the page handler filters to the bound group for the page's Data section.

## 7. RBAC & flags

- No new nodes: wiki-type/category management rides `wiki.manage`; field
  editing rides `wiki.fields.*` + `custom_field_permissions` (unchanged).
- `wiki.enabled` remains the master switch. A type with `enabled=false`
  hides its destinations without deleting pages.

## 8. Migration / rollout

1. Create `wiki_type_configs`, `wiki_type_parents`, `wiki_type_categories`.
2. Seed configs for `country`, `state`, `city`, `university`; seed parent
   edges (`state→country`, `city→state`; city lists under country via the
   state chain — no direct edge). Universities parent under all geo levels.
3. Seed `wiki_type_categories` from the two hardcoded slices, trimmed per
   level — `visa-routes` + `banking` are country-only (national topics);
   state/city seed `overview`, `cost-of-living`, `healthcare`, `community`,
   `housing`, `other`. Delete the Go slices.
4. Seed one empty `custom_field_groups` row per category per level
   (`wiki_{type}_{category}`, `is_system`) + the `space_type:{slug}`
   assignment; `field_group_id` is NOT NULL with `ON DELETE RESTRICT`.
   Field definitions (e.g. `rent_1br`, `meal_inexpensive`) are added by
   admins/ops after rollout — the schema container ships first.
5. Update `GET /wiki/categories` + page handler; admin Wiki section gains
   Types and Categories CRUD screens.

## 9. Non-goals

- Per-entity field overrides (a one-off field on one country) — assignments'
  `entity_id` scoping already covers exceptions.
- Moving child-entity pages (`data_entity_type`) into the category model —
  they coexist: a category either binds a group on the space itself or lists
  child entities; combining both on one page is a later decision.
- Backfilling structured values — ingestion/AI fill via the ops path after
  groups exist.

## 10. Decisions (resolved during implementation)

- **Slug/category immutable after create** — page URLs are
  `/wiki/{entity}/{slug}` and `wiki_pages.category` stores the token;
  renames would break links + history. `label` stays editable.
- **No direct city → country parent edge** — cities list under countries
  through the state chain, matching `space_parent_spaces` usage.
- **Bound groups cannot be deleted** — `ON DELETE RESTRICT` turns group
  deletion into `DeleteFieldGroup`'s existing 409 in_use while a category
  references it. Category deletion never removes its group (fields/values
  may be shared or valuable); orphaned groups are cleaned up manually in
  the Admin Panel.
