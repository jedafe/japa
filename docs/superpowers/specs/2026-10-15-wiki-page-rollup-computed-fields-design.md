# Wiki Page-Scope Rollup & Computed Fields — Sub-Page Aggregation for Category Pages

**Date:** 2026-10-15
**Status:** Design (approved direction)
**Builds on:** §06 (Universal Custom Fields), §80 (Destination Wiki, Wiki Types, Itemized Pricing, Wiki Topic Pages), migrations 000739/000750/000775/000856
**Scope:** `internal/wiki`, `internal/customfields`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§80 amendment notes, catalogs)

## 1. Problem

The §80 rollup computes a category page's aggregates across **descendant
spaces** (country → states/cities, state → cities). A city is the leaf of
the cost-of-living geo chain: it has no descendant spaces carrying
`cost_of_living` categories, so a city Cost of Living page can never
render a rollup — its values can only be hand-entered cfv on the
`wiki_city_cost_of_living` group.

Meanwhile the city's real cost data lives on its **sub-pages**: the
`cost_of_living` category's page templates (`prices` and, after this
spec, the topic pages that nested under it — `food-groceries-prices`,
`restaurant-prices`, `transportation-prices`, `utilities-prices`,
`lifestyle-prices`), each bound to its own shared group whose values are
also stored as cfv on the same space entity. The category page and its
sub-pages are two disconnected surfaces of one dataset; the page axis is
unmodelled ("the system aggregates across spaces, not across pages
within a space").

Two consequences:

1. **No sub-page aggregation.** A city Cost of Living page cannot show
   "average across its cost pages" without duplicating values by hand.
2. **Manual double-entry.** Summary fields on `wiki_city_cost_of_living`
   (`rent_1br`, `exp_food`, `exp_transportation`, `exp_utilities`,
   `exp_internet`) re-state facts the itemized sub-pages already carry —
   the divergence hazard the 2026-10-02 spec flagged on summary/item
   splits.

A second, smaller problem: `prices` is a redundant intermediate level.
Once the category page itself aggregates, "Cost of Living in {City}" and
"Prices in {City}" answer the same question; the topic pages should hang
directly under the category.

## 2. Goals

1. **Page-scope rollup.** A category may set `rollup.mode = "pages"`:
   its page renders a rollup whose rows are the category's sub-pages
   (page templates), aggregated per field key — same read-time,
   never-persisted contract as the space rollup.
2. **Configured source list.** `rollup.pages[]` names the template
   slugs that feed the rollup; omitted → all templates under the
   category.
3. **Computed fields.** A custom field may declare
   `options.computed` — its displayed value is an aggregate of source
   fields (group_slug + key), computed at read time. Computed fields
   reject manual `set_value` ops.
4. **Flatten the city cost-of-living tree.** Topic pages become direct
   children of `cost_of_living`; the `prices` page template is removed.
5. **Zero write-path or storage changes for rollup inputs.** Values stay
   entity-level cfv on the space; no new tables, no jobs, no caches.

## 3. Non-goals

- Persisting computed values into cfv, or propagating them to consumers
  that read raw cfv (JCE comparison items via 000786, `col_dimension`
  source fields via 000785, the cost-of-living tool). Those keep reading
  stored values; materializing computed values downstream is follow-up
  work.
- Aggregating pages across categories — rollup sources are same-category
  templates. Computed *field* sources, however, are group-keyed and may
  reference any group (e.g. `wiki_city_housing` for accommodation costs,
  since housing is a separate seeded category).
- Weighted averages, medians, or per-sample confidence — metrics are the
  existing `min`/`avg`/`max`/`sum` set.
- Editing `options.computed` through a dedicated admin form — it is
  plain options JSONB, editable via the existing custom-fields admin.

## 4. Rollup config

`wiki_type_categories.rollup` gains one mode and one key:

```json
{
  "enabled": true,
  "mode": "pages",
  "metrics": ["min", "avg", "max"],
  "pages": ["food-groceries-prices", "restaurant-prices",
            "transportation-prices", "utilities-prices",
            "lifestyle-prices"]
}
```

- `mode: "pages"` — aggregate over the category's own page templates
  instead of descendant spaces. Joins `"directory"` and the default
  (aggregate-over-descendants) mode. `levels` is ignored in pages mode;
  `pages` is ignored in other modes.
- `pages` — template slugs under this category. Nested-vs-top-level does
  not matter: template resolution is slug-based within the category.
  Empty/absent → every template under the category, in position order.
  Unknown slugs are skipped with a warn log, never fatal.
- `section_limit` still caps rows when configured (a category with many
  templates). `metrics` defaults to all three.

`WikiRollupConfig` gains `Pages []string `json:"pages,omitempty"``.

## 5. Backend — page rollup (`internal/wiki/service_hierarchy.go`)

New `computePageRollup(ctx, userID, spaceID, spaceSlug, spaceName, cfg, templates)`:

1. **Rows = resolved templates.** Map `cfg.Pages` slugs onto the
   category's template list (already fetched for `page_templates` at
   both call sites); empty `pages` → the full list. Each row's display
   title is `section_label` when set, else the `{name}`-substituted
   label.
2. **Fields = union of source group schemas.** For each resolved
   template's `field_group_id`: `ListFieldsByGroup` filtered by
   `rollupFieldTypes`, `section != quick_facts`, and caller `can_view`
   (same `visibleFieldIDs` fail-closed gate as `computeRollup`). Defs are
   keyed by field ID; per-key first-def-wins, in field position order —
   identical to the space rollup.
3. **Values = the space's cfv, one read.**
   `ListFieldValuesWithDefsForEntities(ctx, "space", [spaceID])` —
   every field value already lives on the space entity; each field ID
   belongs to exactly one template's group, so the per-template value
   buckets fall out of the def→template mapping.
4. **Aggregates reuse `aggregateRollupValues` unchanged** — its fold is
   keyed on field key and agnostic to the row axis.
5. **Rows emit for every resolved template** (values may be empty — the
   row doubles as the drill-down link; cells render `—`). Ordered by
   `pages` config order when set, else template position order.
   `section_limit` caps the row count when configured (no default —
   the count is already bounded by the template list).

DTO changes:

```go
// WikiRollupPageRefDTO identifies a sub-page rollup row — the link
// target /wiki/{space}/{slug} plus its display title.
type WikiRollupPageRefDTO struct {
    Slug  string `json:"slug"`
    Title string `json:"title"`
}

type WikiRollupRowDTO struct {
    Space  WikiSpaceRefDTO         `json:"space"`            // host space — populated in both modes
    Page   *WikiRollupPageRefDTO   `json:"page,omitempty"`   // set only in "pages" mode
    Values map[string]any          `json:"values"`
}

type WikiRollupDTO struct {
    Scope  string              `json:"scope,omitempty"` // "pages" when page-scoped; omitted = spaces
    Fields []WikiRollupFieldDTO `json:"fields"`
    Rows   []WikiRollupRowDTO   `json:"rows"`
    Spaces int                  `json:"spaces"`          // contributing rows' source count
}
```

`Space` stays populated (the host space) in pages mode for shape
stability; `page` presence is the row discriminator. `scope` is emitted
so the frontend picks labels without sniffing rows.

Wiring — in `GetWikiPage` (materialized path) and `lazyCategoryPage`,
the existing rollup block gains a third branch:

```go
if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && page.Slug == cat.Slug {
    switch {
    case rcfg.Mode == rollupModeDirectory:
        dto.Directory = s.computeDirectory(...)
    case rcfg.Mode == rollupModePages:
        dto.Rollup = s.computePageRollup(ctx, userID, spaceID, spaceSlug, spaceName, rcfg, templates)
    default:
        dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
    }
}
```

`lazyCategoryPage` gets the same switch minus the `page.Slug` gate —
that path is already the category stub by construction.

## 6. Backend — computed fields

### 6.1 Spec (`internal/customfields`)

```json
"computed": {
  "metric": "avg",
  "sources": [{"group": "wiki_city_prices_food_groceries", "key": "monthly_total"}]
}
```

- `metric`: `avg` (default), `min`, `max`, `sum`.
- `sources`: one or more `{group, key}` pairs — a field in a group,
  resolved via `GetFieldGroupBySlug` + `ListFieldsByGroup`. Sources may
  reference **any** group, not just category templates (e.g.
  `wiki_city_housing`). Unknown groups/keys contribute nothing.
- `customfields.ComputedSpecFromOptions(opts json.RawMessage) *ComputedFieldSpec`
  — same convention as `CurrencyCodeFromOptions` /
  `PrimaryPartFromOptions`: nil on absent/malformed.
- Eligible field types: the existing `rollupFieldTypes` set (number,
  currency, percentage, price_range, price_parts). Emitted value shape
  matches the field type's storage column so every existing renderer
  works unchanged: scalar types emit the bare number;
  `price_range` emits `{"avg": x}` (and `{min,max}` when the metric
  yields them — metric `avg` → avg only); `price_parts` emits
  `{"parts": {primary_part: x}}` using the field's own
  `options.primary_part` (no primary_part → computed spec ignored).

### 6.2 Read path (`internal/wiki`, new `service_computed.go`)

`applyComputedFields(ctx, userID, dtos, valuesByField, visibleSet, canManage)`:

- For each `EntityFieldDTO` whose options carry a spec: resolve each
  source's group slug → group ID (`GetFieldGroupBySlug`, memoized per
  call), look up the space's value in the already-loaded cfv rows
  indexed by `field_group_id + field_key` (the value rows carry both,
  so no per-source field-def query is needed), drop sources the caller
  cannot `can_view`, aggregate by metric.
- Result → `dto.Value` in the field type's native shape (§6.1);
  `dto.Computed = true`; `dto.CanEdit = false`.
- Zero usable sources → `Value = nil`. **Never falls back to the stored
  cfv** — the stored row stays in the DB untouched, so removing the spec
  restores manual entry, but a declared-computed field never shows a
  hand-entered value under the computed badge.

Call sites — both keep their existing loaded values, so the helper adds
no queries beyond source-def resolution (memoized):

- `GetPageFields` — covers the materialized page, lazy category page,
  lazy template page, and composite preview sections in one shot.
- `GetEntityFields` — the edit/schema surface reports the same
  read-only computed value.

`EntityFieldDTO` gains `Computed bool `json:"computed,omitempty"``.

### 6.3 Write path

- `checkSetValueFields` (submit time): a `set_value` op whose target
  schema field's options carry a `computed` spec is rejected
  (`ErrOpNotPermitted`-class error — the field is not editable, period;
  `wiki.fields.manage` does not bypass).
- `applySetValue` (approve time): same check on the transactional schema
  — a revision queued before a spec was added cannot slip a manual value
  in at apply time.
- `EntityFieldDTO.CanEdit = false` removes the affordance from
  `EntityFieldsPanel`/`SuggestEdit` without frontend changes.

## 7. Backend — flatten the city cost-of-living tree

Migration `000857_wiki_city_cost_of_living_page_rollup.up.sql`:

1. Re-parent the five topic templates to top-level:
   `UPDATE wiki_type_page_templates SET parent_template_id = NULL` for
   the 000775-seeded slugs under the city `cost_of_living` category.
2. Delete the `prices` template row (city `cost_of_living`). Safe:
   - `wiki_pages.page_template_id` is `ON DELETE SET NULL` — a city that
     materialized `/wiki/{city}/prices` keeps the row as an ad-hoc page
     bound to `wiki_city_prices` (renders prose + any group fields).
   - Unmaterialized lazy `/prices` URLs then 404 — acceptable; the
     content was the composite previews now living on the category page.
   - `wiki_city_prices` group and the `city-prices` JCE comparison type
     are untouched — the flat city↔city comparison keeps working.
3. Set the city `cost_of_living` rollup:
   `{"enabled": true, "mode": "pages", "metrics": ["min","avg","max"],
     "pages": ["food-groceries-prices","restaurant-prices",
               "transportation-prices","utilities-prices",
               "lifestyle-prices"]}`.
4. Down migration restores `parent_template_id` (via the `prices`
   template's id when present), re-inserts the `prices` template
   (idempotent seed shape), and sets `rollup = NULL`.

**Composite previews on the category page.** `attachCompositeSections`
currently runs only for template-bound pages (`pageTmpl != nil`). Extend
it to the category stub page with a new `topLevelTemplates(templates)`
helper (templates with an invalid `ParentTemplateID`) — note
`childTemplates(templates, pgtype.UUID{})` does NOT work: its
invalid-parent guard returns nil because a nil parent means "no
children" for a template page, not "the top level". This preserves the
one-screen "all itemized prices" view the `prices` page used to provide
— now directly on Cost of Living.
Guard: skip `is_system` templates whose group produced zero visible
fields? No — keep the existing always-emit behavior (the section is the
drill-down link); groups are empty containers until admins add fields,
and an empty section is already tolerated.

## 8. Frontend (`apps/main`)

- `types/index.ts`: `WikiRollup.scope?: string`, `WikiRollupRow.page?:
  { slug: string; title: string }`, `EntityField.computed?: boolean`.
- `WikiRollupPanel.svelte`:
  - `row.page` present → row label links to
    `/wiki/{row.space.slug}/{row.page.slug}` with `page.title`; first
    column header reads `wiki.rollup_col_page` ("Page"); footer reads
    `wiki.rollup_scope_pages` ("Aggregated from {count} cost pages").
  - The `{#each rollup.rows}` key must become
    `(row.page?.slug ?? row.space.slug)` — page rows share the host
    space's slug, so the current `row.space.slug` key collides.
  - Compare rail needs no change — `type_slug` is empty/absent on page
    rows, `compareTarget` resolves null, checkboxes don't render.
  - Summary cards, estimated-total donut, and cell formatting are
    field-type-driven — unchanged.
- `WikiItemTables` / field renderers: `computed` fields show a small
  "computed" badge (i18n `wiki.field_computed`) — informational only;
  edit affordances are already gated by `can_edit`.
- i18n keys in `en.json`/`fr.json` (main app).

## 9. Admin (`apps/admin`)

- Wiki category editor (`community/wiki/+page.svelte`): the rollup mode
  picker gains "Pages — aggregate this category's sub-pages"; selecting
  it swaps the levels checklist for a `pages` multi-select populated
  from the category's own templates. Payload writes `mode`/`pages`
  through the existing `rollup` JSON passthrough — no API change.
- i18n en/fr strings (`app.wiki.rollup_mode_pages*`, `app.wiki.rollup_pages*`).

## 10. Data flow — read

```
GET /wiki/{city}/cost-of-living
  └─ GetWikiPage / lazyCategoryPage
       ├─ cat (wiki_city_cost_of_living group) → GetPageFields
       │     └─ applyComputedFields: field options.computed
       │          → source groups (memoized) → cfv valuesByField
       │          → can_view gate → metric → dto.Value + computed:true
       ├─ rollup.mode=="pages" → computePageRollup
       │     └─ cfg.Pages → templates → groups → rollupFieldTypes
       │          → cfv (1 query) → aggregateRollupValues (unchanged)
       │          → rows = {space: host, page: {slug,title}, values}
       └─ composite sections ← top-level templates (flattened topics)
```

## 11. Edge cases

- **Key collision across pages** (e.g. two topic groups both define
  `monthly_total`): aggregates fold across every page carrying the key —
  `samples` reflects the count; row cells show each page's own value.
- **Field not visible to the reader**: excluded from aggregates and row
  cells; a computed field's invisible sources contribute nothing
  (fail-closed, matching the space rollup).
- **Non-finite numeric** (`NaN` in a numeric cfv): `finite()` guard in
  the shared fold already drops it.
- **Template listed in `pages` but deleted later**: skipped with warn —
  same tolerance as `levels` slugs in the space rollup.
- **`pages` mode on a category with no templates**: `computePageRollup`
  returns nil; the page renders its own fields only.
- **Computed spec on a field with no sources / all sources empty**:
  `Value = nil` — renders `—`, still `computed: true`, still uneditable.
- **Ad-hoc materialized pages under the category**: not rollup rows —
  only declared templates feed the configured list.

## 12. Testing

- `computePageRollup`: configured vs empty `pages`; nested-template
  resolution; per-key metrics; `quick_facts`/invisible-field exclusion;
  empty-values rows still emitted; deterministic ordering.
- `applyComputedFields`: avg/min/max/sum; missing group, missing key,
  zero sources; `can_view` filtering; `can_edit=false` + `computed` on
  the DTO; value shape per field type (number / price_range /
  price_parts).
- `checkSetValueFields` + `applySetValue`: `set_value` on a computed
  field rejected at submit and apply; unaffected ops still pass.
- Category-page composite sections: top-level templates emit sections;
  nested (none remain after 000857, but keep the guard honest) excluded.
- Migration: up applies cleanly on a seeded DB; down restores the prior
  shape.

## 13. Follow-ups (not in this spec)

- Seed `computed` specs on `wiki_city_cost_of_living` fields once the
  topic groups carry item fields (they are empty containers today).
- Consume computed values in JCE comparison items / `col_dimension`
  sources / the cost-of-living tool (currently read stored cfv).
- Optional `include_self` flag to add the category's own group as a
  rollup row.
