# Wiki Topic Pages — Nested Page Templates, Composite Section Previews, and Multi-Part Price Fields

**Date:** 2026-10-02
**Status:** Design
**Builds on:** §06 (Universal Custom Fields), §80 (Destination Wiki + Community Field Ops + Tool Surface Hydration + Wiki Types + Itemized Pricing), §30 (JCE), 2026-09-26 wiki-types-and-field-group-pages design, 2026-09-28 wiki-hierarchical-ux-and-itemized-pricing design, migrations 000739/000750
**Scope:** `internal/wiki`, `internal/customfields`, `internal/comparison`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§80 amendment, catalogs)

## 1. Problem

The 2026-09-28 design delivered sectioned item tables: a page template
(`prices` under `cost_of_living`) binds a shared type-level group
(`wiki_city_prices`), `custom_fields.section` groups its fields into item
tables, and `?section={token}` is the same-page "View more" target. One
level of the target UX remains unmodelled — the **topic page** (e.g.
"Transportation Prices in {City}"):

- **It is a real page, not a filtered view.** The mockup shows its own
  title, byline, prose intro, tips, FAQs, revision history, a breadcrumb
  segment (`… › {space} › Cost of Living › Prices in {City} ›
  Transportation Prices`), and Prev/Next across *sibling topic* pages
  (Utilities Prices ⇄ Food & Groceries Prices). `?section=` cannot carry
  any of that — it keeps the parent's title, prose, and history.
- **It needs a second grouping level.** "Transportation" splits into
  sub-tables (Public Transportation (TTC), Taxis & Rideshare, Car
  Ownership, Biking & Micromobility) — one level deeper than
  `custom_fields.section` provides *on the prices group*.
- **Tables have heterogeneous column schemas.** Taxis render
  `Service | Base Fare | per km | per min | Minimum Fare`; TTC renders
  `Item | Price | Details`. `price_range` only models `{avg,min,max}` —
  there is no named-parts composite value.
- **The parent page's section previews must not duplicate data.** If
  `wiki_city_prices` keeps a `transportation` section of summary fields
  while a topic group holds the real items, the same fact ("Monthly Pass
  CAD 156") lives under two field keys and diverges — the exact failure
  §14 of the prior spec flagged on the summary/item split.

## 2. Goals

1. **Topic pages are real (lazy) wiki pages.** Each topic under a
   template is itself a template: own slug, own bound group, own prose,
   byline, history, feedback — everything a page already is.
2. **Composite parent pages, zero duplication.** A template with
   children renders its own group's fields plus one *preview section*
   per child — rows drawn from the child group's own fields+values.
   Every item is stored once, in exactly one group, under one cfv key.
3. **Page-level ancestry.** The page response carries the page ancestor
   chain (category stub → template parents) so the frontend renders the
   full breadcrumb and a "Back to {parent}" link.
4. **Sibling nav scoped by parentage.** Topic pages Prev/Next across
   their sibling topics; top-level pages wrap through the category stub.
5. **Multi-part item values.** A `price_parts` field type stores named
   numeric/text parts so one field renders as one row of a
   heterogeneously-columned table.
6. **Zero divergence from the §80 contract.** cfv stays the store of
   record; topic pages are lazy; writes flow through the ops queue.

## 3. Design — Nested page templates

```sql
ALTER TABLE wiki_type_page_templates
  ADD COLUMN parent_template_id uuid REFERENCES wiki_type_page_templates(id) ON DELETE SET NULL,
  ADD COLUMN section_label      text,        -- short label when rendered as a parent section ('Transportation'); falls back to label
  ADD COLUMN preview_limit      int  NOT NULL DEFAULT 5, -- rows of the child group shown on the parent; 0 = all
  ADD CONSTRAINT wiki_page_templates_no_self_parent CHECK (parent_template_id <> id);
```

- **One nesting level (v1).** `parent_template_id` must reference a
  template of the **same category** that is itself top-level
  (`parent_template_id IS NULL`) — validated at the service layer with
  `400 invalid_parent_template`. Depth ≤1 by construction: no cycles,
  no deeper chains. Deeper nesting adds no URL depth (the URL space is
  flat — `/wiki/{space}/{slug}`), only breadcrumb/section semantics,
  and is deferred.
- **`UNIQUE (category_id, slug)` is unchanged** — topic slugs share the
  category's slug space, so `transportation-prices` resolves through the
  existing `GetPageTemplateBySlug` path with zero dispatcher changes.
  Lazy rendering, materialization on first submission, template-shadows-
  category precedence, and deletion guards all apply as-is.
- **`ON DELETE SET NULL` promotes orphans.** Deleting a parent template
  lifts its children to top-level — their bound pages keep working
  (same convention as `wiki_pages.page_template_id`).
- **`section_label`** is the label used when the child renders *as a
  section* on the parent ("Transportation"), distinct from the child's
  page label ("Transportation Prices in {name}"). Unset → the parent
  section uses the child's `{name}`-substituted `label`.
- Group provisioning is unchanged: a nested template create
  auto-provisions `wiki_{type}_page_{slug}` + the `space_type:{slug}`
  assignment. Each topic group's fields carry `section` tokens for its
  sub-tables (`ttc`, `taxis_rideshare`, `car_ownership`, `biking` on
  `wiki_city_prices_transportation`) — the existing section machinery
  IS the second grouping level.

### Rejected alternatives

- **Upgrade `?section=` into a virtual page** (+ a `subsection` column):
  cheapest, but a query-filtered view has no own prose, byline,
  revision history, canonical slug, or feedback target — the mockup
  shows all of them.
- **Duplicated preview fields on the parent group:** no schema work, but
  the same item exists under two field keys across two groups — values
  diverge, ops updates hit only one, and JCE sees double dimensions.
- **`data_entity_type` child entities for topics:** rejected in the
  prior spec — reintroduces per-space instance rows and an
  item-dictionary governance problem.

## 4. Design — Composite sections (the parent preview)

When a page's bound template has children, its `fields.sections[]`
gains one **composite section** per child template (ordered by child
`position`), appended after the page's own group's sections:

```json
"fields": {
  "id": "…", "slug": "wiki_city_prices", "name": "Prices (city)",
  "fields": [ /* own group's fields — summary/page-level items */ ],
  "sections": [
    { "key": "", "fields": [ /* own unsectioned fields */ ] },
    { "key": "transportation-prices",
      "label": "Transportation",
      "page_slug": "transportation-prices",
      "fields": [ /* first preview_limit visible fields of
                     wiki_city_prices_transportation, with the host
                     space's cfv values */ ] }
  ]
}
```

- `key` = the child template's slug; `label` = `section_label` fallback
  `{name}`-substituted `label`; `page_slug` marks the section as
  composite and is the drill-down target — the frontend renders
  "View more {label} prices →" as `/wiki/{space}/{page_slug}` instead of
  `?section={key}`.
- **One field, one store.** Preview fields are the child group's real
  definitions; values are the space's cfv for that group — the same
  `GetPageFields` merge, batched across child groups in one cfv query
  (`entity_type='space'`, `entity_id=space.id`). Field-level RBAC and
  `show_in_frontend` apply identically.
- `preview_limit` bounds rows (`0` = all); the "View more" link renders
  whenever `page_slug` is present — a topic page is discoverable even
  when its preview is short.
- `?section={key}` continues to filter the rendered section list —
  including composite sections (the same-page inline view stays as a
  non-canonical convenience); plain sections carry neither `label` nor
  `page_slug` and behave exactly as today.
- The parent's own group stays for page-level/summary fields
  (`estimated_total_monthly`, currency notes). `wiki_city_prices`
  (seeded in 000750) keeps that role — item fields live in topic groups.

## 5. Design — Page ancestry (`parents[]`)

`GET /wiki/:space_slug/:page_slug` gains `parents[]` — ordered page
ancestors, each `{slug, title}`:

```
category page stub  →  { slug: 'cost-of-living', title: 'Cost of Living' }
template ancestors  →  { slug: 'prices',         title: 'Prices in Toronto' }   (root → immediate parent)
```

- Emitted for every page whose category is managed: `[category stub]` on
  a top-level page, `[category stub, …template chain]` on a topic page,
  `[]` on the category stub page itself (it IS that segment).
- Resolution: page → `page_template_id` → walk `parent_template_id`
  (depth ≤1 in v1) → prepend the category's stub (`{slug, label}`).
  Materialized pages whose template was deleted (`page_template_id`
  NULL'd) keep `[category stub]`; ad-hoc pages likewise.
- Frontend: breadcrumb becomes
  `Wiki › {type} Guides › {ancestors…} › {space} › {parents…} › {page.title}`;
  a `← Back to {parents[-1].title}` link renders above the article when
  `parents` is non-empty.

## 6. Design — Sibling scoping

`siblings[]` becomes **same-parentage**:

- **Nested page:** its parent's child *templates* — ordered `position`,
  each `{slug, title, position, current}`. Lazy templates are included
  (they resolve at the URL), so Prev/Next spans the full declared topic
  set regardless of materialization.
- **Top-level page:** the category stub (position 0), top-level
  templates, and materialized ad-hoc pages — union ordered `position,
  created_at, slug`. This is what makes "Cost of Living in {City}" the
  natural Prev of "Prices in {City}" (image 5) with no wrap rule.
- Ad-hoc pages keep today's materialized-only semantics when they sit in
  an unmanaged category.

## 7. Design — `price_parts` field type

A new proposable field type for heterogeneous item rows. One field = one
row; its parts are the columns.

- `value_json`: `{"parts": {"<part_key>": <number|string>, …}}`.
- `options.parts`: `[{"key":"base_fare","label":"Base Fare","kind":"money"}, …]`
  — the column schema. `kind ∈ money|number|text`; `money` formats
  through the existing currency chain (`options.currency_code` → host
  space's country currency → bare number).
- Rendering: a section whose fields are all `price_parts` renders a
  parts table — columns from the first field's `options.parts`, `—` for
  parts a row doesn't declare. Sections mixing `price_parts` with other
  types fall back to the definition-list render. `options.emphasize:
  true` marks a total row (e.g. "Total Estimated Monthly Cost").
- Ops: `price_parts` joins `ProposableFieldTypes`; `set_value` coercion
  validates part keys against `options.parts` and coerces
  `money`/`number` parts to numeric — unknown keys are rejected, not
  silently dropped.
- **JCE:** a `price_parts` field projects as a dimension only when
  `options.primary_part` names the comparable part (currency-typed on
  that part); fields without `primary_part` are non-comparable and are
  skipped by rollup aggregation for the same reason.
- **Rollup:** `rollupFieldTypes` treats `price_parts` like `price_range`
  when `primary_part` is declared (aggregating that part); otherwise the
  field contributes to `rows[].values` only.

## 8. Design — Topic-scoped rail cards

- **Quick Facts.** Reserved section token `quick_facts` on a bound
  group: its fields never render a body table — they render in the
  right-rail Quick Facts card, scoped to the page's bound group (topic
  facts on topic pages, overview facts on index pages). `WikiQuickFacts`
  gains an optional `groupSlug` prop fed by
  `data.page.field_group_slug`: it prefers the bound group's
  `quick_facts`-section fields, falling back to today's flatten-all
  behavior when the section is absent. No API change — the data already
  rides `GET /wiki/entities/:type/:id/fields` (`groups[].sections`).
- **ON THIS PAGE.** Frontend-only TOC card in the right rail listing the
  page's `fields.sections[]` anchors (`#section-{key}` is already
  emitted), plus `wiki-rollup`/`wiki-directory` when those panels
  render. Shown when the page has ≥2 sections.
- Tips/FAQ/CTA chrome in the mockups is prose (`Tip:`/`Note:` callouts,
  the existing question/community cards) — no machinery.

## 9. API surface (additions only — no new routes)

```
GET  /api/v1/wiki/:space_slug/:page_slug
     gains: parents[]; fields.sections[] entries gain label?, page_slug?;
            siblings[] semantics → same-parentage (§6)

POST /api/v1/admin/wiki/categories/:id/page-templates
     gains: parent_template_id?, section_label?, preview_limit?
PATCH /api/v1/admin/wiki/page-templates/:id
     gains: same three fields, three-state (absent keeps, null clears)
GET  /api/v1/admin/wiki/categories/:id/page-templates
     rows gain: parent_template_id, section_label, preview_limit
GET  /api/v1/wiki/categories?space=
     page_templates[] items gain parent_template_id (frontend nests
     "More in this section"); composite topics are reachable through
     their parent page and remain lazy-resolvable directly.
```

`GET /wiki/entities/:type/:id/fields` is unchanged — the topic group's
values are ordinary cfv. Ops vocabulary is unchanged (`price_parts` is a
type, not an op).

## 10. RBAC, flags, guards

- Nested-template CRUD rides `wiki.manage`; `price_parts` editing rides
  `wiki.fields.*` unchanged. No new nodes, no new flags.
- Parent validation is fail-closed: cross-category parents, self-parents
  (also blocked by the CHECK), and parents that are themselves nested →
  `400 invalid_parent_template`.
- Existing template deletion guards apply to children identically
  (blocked while bound pages carry prose/values). Deleting a *parent*
  never deletes children — `SET NULL` promotes them.
- Composite previews inherit the fail-closed field-RBAC note from the
  rollup work: fields added to topic groups need `can_view` grants for
  guest/member groups or sections render empty.

## 11. Rollup & JCE interaction

- Rollup source fields already union "the category group + every
  page-template group nested under that category" — nested topic
  templates are under the same category, so their groups join the union
  automatically. `ListPageTemplates` must return the full template set
  of the category, not only top-level rows.
- JCE binds topic groups directly — `city-transportation-prices`
  comparison type on `wiki_city_prices_transportation` yields
  "Toronto vs Vancouver transportation" with no `config.sections`
  filter. The `config.sections` filter remains for within-group
  scoping; both mechanisms coexist.
- Level-purity is preserved: `wiki_city_prices_*` groups are still
  per-type; cross-level comparison stays unrepresentable.

## 12. Migration / rollout

1. Migration: `wiki_type_page_templates` columns + CHECK; register
   `price_parts` (type registry, coercion, `options.parts` validation,
   `ProposableFieldTypes`).
2. Backend: composite section synthesis (child groups batched into the
   page DTO), `parents[]`, same-parentage `siblings[]` (incl. lazy
   template siblings), admin CRUD fields + validation.
3. Frontend (`apps/main`): `page_slug`-aware "View more" links,
   `parents[]` breadcrumb segments + "Back to parent", `price_parts`
   table renderer, `quick_facts` rail scoping, ON THIS PAGE card,
   nested "More in this section".
4. Admin (`apps/admin`): parent/section_label/preview_limit on the
   template form (parent picker = same-category top-level templates);
   `price_parts` parts editor on the field form.
5. JCE: `price_parts` `primary_part` projection; topic-group comparison
   type seeds.
6. Seeds (city `cost_of_living` → `prices` children, `preview_limit`
   tuned per topic): `food-groceries-prices`, `restaurant-prices`,
   `transportation-prices`, `utilities-prices`, `lifestyle-prices` —
   each auto-provisioning `wiki_city_prices_{topic}` +
   `space_type:city` assignment. Topic groups ship as empty schema
   containers; fields arrive via admin/ops as before.

## 13. Non-goals

- Nesting beyond one level — breadcrumbs/templates are built so depth
  can grow later, but v1 validates parents as top-level-only.
- Deriving summary fields from item data — still entered per the prior
  spec's decision.
- Related Articles curation, Save/Share chrome — existing/frontend
  widgets, not wiki machinery.
- Topic pages on non-city types — the machinery is type-agnostic; seeds
  are a data decision per type.
- `data_entity_type` child-entity pages — unchanged, they coexist.

## 14. Decisions

- **Nested templates + composite previews** chosen over the `?section=`
  virtual page (no page identity) and duplicated summary fields
  (divergence). The parent page is a *view over its children*, not a
  data store.
- **Section ↔ child template is a structural link** (`page_slug` on the
  synthesized section), not a naming convention — no slug-parsing, works
  for any labels/locales.
- **`?section=` is superseded as the canonical drill-down** for
  composite sections (the 2026-09-28 spec's "no new route" note applied
  to single-level groups; topic pages are new *pages*, not new routes).
  It remains the inline filter and the fallback for plain sections.
