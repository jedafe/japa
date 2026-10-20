# JCE ↔ Wiki Field-Group Mapping — One Registry for `/compare` and `/wiki`

**Date:** 2026-10-20
**Status:** Design (direction approved; binding + subpage decisions locked)
**Builds on:** §06 (Universal Custom Fields), §80 (Destination Wiki), 2026-09-26 wiki-types-and-field-group-pages design, 2026-10-02 wiki-topic-pages design, 2026-10-15 wiki-page-rollup-computed-fields design, migrations 000739/000750/000775/000856–858
**Source of truth:** `docs/canva/jce-to-wiki-pages-fieldgroup-mapping/jce-to-wiki-field-group-mapping.mindmap`
**Scope:** `internal/wiki`, `internal/comparison`, `internal/customfields`, `internal/spaces`, `internal/visaroutes`, `internal/scholarships`, `internal/organizations`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§80 amendment, catalogs)

## 1. Problem

`/compare` (JCE) and `/wiki` (Destination Wiki) maintain the same
structured-data model in parallel:

- **Comparison types bind ad-hoc groups.** `comparison_type_definitions.field_group_id`
  points at comparison-owned groups (`geo_fields`, `bank_account_registration_fields`,
  `residents_permit_requirements_fields`, …) while the wiki maintains the
  canonical per-type groups (`wiki_{type}_{category}`). Two schemas for the
  same facts — they drift, and fields authored for one surface are invisible
  to the other.
- **Wiki is space-only.** `wiki_pages.space_id` is `NOT NULL`; every wiki
  type is a `space_type`. But the comparison engine already resolves
  **non-space entities** — `visa_route`, `scholarship`, `course`,
  `col_city`, `organization` (banks), `mentor` — which have real tables
  (`visa_routes`, `scholarships`, `organizations`,
  `university_course_offerings`) and already merge `custom_field_values`
  over their native payloads (§80 Tool Surface Hydration). These entities
  cannot get wiki pages today, so the map's `visa route`, `bank`,
  `embassy`, `scholarship`, `course` branches are unimplementable.
- **The type→subtype→field-group contract is implicit.** Migration 000775
  noted a topic-group comparison type can project `wiki_city_prices_transportation`
  directly — but nothing declares *which* compare leaf binds *which* wiki
  group. The mindmap formalizes that contract; this spec turns it into rows.

## 2. Goals

1. **One sourcing contract.** Every comparison resolves to
   `(wiki type / level) × (field group) × (≥2 items)`.
   `comparison_type_definitions.field_group_id` always references a
   wiki-owned group — never a parallel comparison-owned group.
2. **Polymorphic wiki binding.** `wiki_pages` binds to a space **or** an
   entity row (`entity_type` + `entity_id`), so entity kinds get full wiki
   structure: categories, topic pages, field ops, rollups.
3. **Subpages own dedicated groups** (decision locked). Topic pages keep
   their per-template groups (`wiki_{type}_{cat}_{sub}`) — no
   `config.sections` slicing for subtype compares.
4. **Child wiki types at any depth** via `wiki_type_parents`
   (`country → visa route → student visa`; `university → course`).
   Child types may inherit the parent's category template with
   per-type groups.
5. **Family-grouped compare selector.** The `/compare` type selector groups
   types into families (`location`, `education`, `visa routes`, `banking`,
   `embassies`) plus entity (category) types — matching the mindmap.

## 3. Non-goals

- Field *value* population — existing community edits + AI field ops
  continue to write `custom_field_values`; unchanged.
- Compare stepper/UX redesign beyond family grouping and subtype
  narrowing.
- cfv storage, field-ops pipeline, rollup engines — all unchanged.

## 4. Domain model (mindmap vocabulary → platform vocabulary)

| Mindmap term | Platform object |
|---|---|
| wiki type | `wiki_type_configs` row — over a `space_type` **or** an entity kind |
| wiki page | `wiki_pages` row — bound to `space_id` or `(entity_type, entity_id)` |
| category | `wiki_type_categories` row → dedicated field group |
| subpage | `wiki_type_page_templates` child (`parent_template_id`) → own group |
| PRIMARY group | the type's `overview` category group — the only group a wiki-type compare may consume |
| compare family | new `config.family` on `comparison_type_definitions` — selector grouping |
| compare subtype | `(level, group)` — level picks the item kind, group picks dimensions |
| the link | `field_group_id` — one row shared by the wiki template and the comparison type |

Resolution rule (the whole model in one line):

> Pick type → narrow to level/subtype → that selects **one field group**
> and the **item kind** → pick ≥2 items → render the group's fields as
> dimensions, hydrated from `custom_field_values`.

Two compare modes:

- **Wiki-type compare** (location, education, visa routes, banking,
  embassies families): the group is fixed to the type's `overview`
  (PRIMARY). The subtype picker only chooses the level within the family.
- **Entity compare** (`[[entity]]` = wiki categories as types): the type
  is a category (Cost of Living, Requirements, Tuition & Fees); the user
  picks the host level (`country|state|city` for COL) and optionally a
  subpage (Transportation) — each selection pins a different group.

Facet vs entity can coexist: `banking` remains a category page on geo
types (the banking-system facet: "how banking works in Nigeria") while
`bank` is an entity kind (GTBank, Chase — compared directly).

## 5. Design — polymorphic wiki binding

`wiki_pages` gains an entity target mirroring the existing
`wiki_revisions (target_entity_type, target_entity_id)` precedent:

```sql
ALTER TABLE wiki_pages
  ALTER COLUMN space_id DROP NOT NULL,
  ADD COLUMN entity_type text,
  ADD COLUMN entity_id   uuid,
  ADD CONSTRAINT wiki_pages_one_parent CHECK (
    (space_id IS NOT NULL)::int + (entity_id IS NOT NULL)::int = 1
  ),
  ADD CONSTRAINT wiki_pages_entity_pair CHECK (
    (entity_type IS NULL) = (entity_id IS NULL)
  );

CREATE UNIQUE INDEX wiki_pages_entity_slug_key
  ON wiki_pages (entity_type, entity_id, slug) WHERE entity_id IS NOT NULL;
```

`wiki_type_configs` generalizes the same way — a wiki type is a
`space_type` **or** a registered entity kind:

```sql
ALTER TABLE wiki_type_configs
  ALTER COLUMN space_type_id DROP NOT NULL,   -- PK moves to surrogate id
  ADD COLUMN id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  ADD COLUMN entity_kind text,                -- 'visa_route','organization','scholarship','course','embassy'
  ADD COLUMN entity_discriminator jsonb,      -- e.g. {"column":"route_type","eq":"student"} for visa kinds
  ADD CONSTRAINT wiki_type_configs_one_binding CHECK (
    (space_type_id IS NOT NULL)::int + (entity_kind IS NOT NULL)::int = 1
  );

ALTER TABLE wiki_type_parents
  ADD COLUMN entity_kind text,              -- mirrors configs; one of (space_type_id, entity_kind)
  ADD COLUMN parent_entity_kind text;       -- parents may themselves be entity kinds
```

`entity_discriminator` lets several wiki types share one entity table:
all six visa kinds are `visa_routes` rows distinguished by
`{"column":"route_kind","eq":"student"}` etc. — so `student visa`,
`work visa`, `family visa`, `tourist visa`, `business visa`,
`pr pathway` are six wiki types over one table, each with its own
`wiki_{kind}_overview` group.

`custom_field_group_assignments.entity_class` already accepts arbitrary
classes; entity-bound groups register as `entity_class = 'visa_route'`
(existing convention is `'space_type:{slug}'` — entity kinds use the
bare kind slug). cfv writes key on the entity row id, which the JCE
resolvers already merge — one store, no sync.

**Entity-kind inventory** (bindings for the mindmap's new types):

| wiki type | binding | source table | JCE resolver |
|---|---|---|---|
| country/state/city/university/attraction | space | `spaces` | `space`/`space_type` ✓ |
| neighborhood | space | `spaces` | `space` (new space_type) |
| visa route + 6 kinds | entity | `visa_routes` (+`route_kind`) | `visa_route` ✓ |
| course | entity | `university_course_offerings` | `course` ✓ |
| scholarship | entity | `scholarships` | `scholarship` ✓ |
| bank | entity | `organizations` (kind=bank) | `organization` — **verify exists** |
| embassy | entity | `monitored_embassies` (or new `embassies`) | **new resolver** |

**Page addressing.** Space pages resolve `/wiki/{space_slug}/{page_slug}`
via space slug. Entity-bound pages need their own route segment —
propose `/wiki/e/{entity_kind}/{entity_slug}/{page_slug}` to keep the
existing `[space_slug]` param unambiguous.

## 6. Design — registry seeds (mindmap → rows)

For every new wiki type: `wiki_type_configs` + `wiki_type_parents` +
`wiki_type_categories` (+ `wiki_type_page_templates` children for
subpages), each category spawning a dedicated
`wiki_{type}_{category}` group (existing seed convention).

**New types** (binding → parents → categories; full sets in mindmap):

- `neighborhood` — space type → parent `city`. Categories: overview,
  safety, rent prices, amenities.
- `visa_route` — entity → parent `country`. Categories: overview
  (PRIMARY), requirements & eligibility, fees & costs, processing time,
  application process.
  - kinds `student|work|family|tourist|business|pr_pathway` — child wiki
    types over `visa_routes` via discriminator; each gets PRIMARY
    overview + inherits the visa-route category template with per-kind
    groups (`wiki_student_visa_requirements`, …).
- `course` — entity (`university_course_offerings`) → parent
  `university`. Categories: overview, entry requirements, tuition &
  fees, career outcomes.
- `scholarship` — entity → parents `university`, `country`. Categories:
  overview, eligibility, coverage & benefits, application & deadlines.
- `bank` — entity (`organizations` where kind=bank) → parent `country`.
  Categories: overview, account types & fees, requirements
  (non-resident), remittance & fx, branch & ATM coverage.
- `embassy` — entity (`monitored_embassies`) → parent `country`.
  Categories: overview, consular services, appointment booking, contact
  & location.

**Geo category expansion** to the map's canonical sets (country 10 /
state 10 / city 12 categories incl. `banking`, `safety & crime`,
`jobs`, `schools`, `transportation & commute`, `utilities`, `weather`,
`attractions`, `community`); city `cost_of_living` topic pages extend to
8 subpages: transportation, food & restaurants, groceries, rent &
housing, utilities, healthcare, childcare & school fees, leisure &
lifestyle. University gains `tuition & fees` subpages
(undergrad/postgrad), rankings, student life, accommodation.

## 7. Design — comparison types bound to wiki groups

One `comparison_type_definitions` row per mindmap compare leaf;
`field_group_id` points at the wiki-owned group. `config` gains:

```json
{
  "family": "location | education | visa_routes | banking | embassies | <category slug>",
  "level": "country",                     // subtype level within the family
  "item_ref_types": ["country"],          // existing
  "scope_chain": [...],                    // existing
  "allow_nested_comparisons": true         // existing
}
```

**Wiki-type families** (PRIMARY-group compares):

| family | subtype (slug) | item_ref | group |
|---|---|---|---|
| location | `countries` | `country` | `wiki_country_overview` |
| | `states` | `state` | `wiki_state_overview` |
| | `cities` | `city` | `wiki_city_overview` |
| | `neighborhoods` | `neighborhood` | `wiki_neighborhood_overview` |
| education | `universities` | `university` | `wiki_university_overview` |
| | `courses` | `course` | `wiki_course_overview` |
| | `scholarships` | `scholarship` | `wiki_scholarship_overview` |
| visa routes | `visa-routes` + 6 kinds | `visa_route` | `wiki_{kind}_overview` |
| banking | `banks` | `organization` | `wiki_bank_overview` |
| embassies | `embassies` | `embassy` | `wiki_embassy_overview` |

**Entity types** (category compares — `family` = the category):

| type | host levels | group | example |
|---|---|---|---|
| `cost-of-living` | country\|state\|city | `wiki_{level}_cost_of_living` | Nigeria COL vs Ghana COL |
| `col-transportation` | city | `wiki_city_prices_transportation` | transport prices Warri vs Accra |
| … (per COL subpage) | city | `wiki_city_{sub}` | |
| `visa-requirements` | per kind | `wiki_{kind}_requirements` | NG vs GH student-visa reqs |
| `admission-process` | university | `wiki_university_admission_process` | |
| `tuition-fees` | university | `wiki_university_tuition` (+subpages) | |
| `safety`, `housing`, `jobs`, … | per-level | `wiki_{level}_{cat}` | |

Existing geo types rebind from `geo_fields` to the `wiki_*_overview`
groups (keep `geo_fields` fields merged into the overview groups or
drop the group once orphaned — migration decision).

**Nested comparisons** keep working: `allowed_nested_types` gains the
new type slugs so a country-vs-country compare can embed visa-route or
COL sub-compares.

## 8. Frontend

**`/compare`** (`apps/main/src/routes/(app)/compare`):
- `TypeCardGrid` groups by `config.family`; selecting a wiki-type family
  narrows to level (country/state/city) then items — group is implicit.
- Entity type selected → host-level picker → optional subtype (subpage)
  picker → items. `ScopeChain`/`NarrowPanel`/`compareFlow.ts` already
  carry scope-selection; extend the chain with the level/subpage step
  (`requiredScopeStep` analogue: `level` for entity types).
- No group picker ever — the group is always implied by (type, level,
  subtype).

**`/wiki`** (`apps/main/src/routes/(app)/wiki`):
- Entity-bound pages route under `/wiki/e/{kind}/{slug}/…`; the
  destinations tree (`ListWikiDestinations`) learns entity kinds so a
  country page shows its visa routes / banks / embassies alongside its
  states and cities.
- Page → Data panel already renders the bound group's cfv — entity-bound
  groups hydrate identically.

**Admin** (`apps/admin`): wiki-type/category CRUD screens must handle
entity bindings (pick entity kind + discriminator instead of space
type).

## 9. Rollout

| Phase | Work | Depends on |
|---|---|---|
| M1 schema | `wiki_pages`/`wiki_type_configs` polymorphism, sqlc regen | — |
| M2 geo registry | neighborhood type; category expansion per map | M1 |
| M3 entity types | visa route + kinds, course, scholarship, bank, embassy configs/categories/groups | M1 |
| M4 JCE seeds | comparison types → wiki groups, `config.family`; rebind geo types off `geo_fields` | M2, M3 |
| M5 resolvers | `embassy`, `organization`/`bank` verify+add; kind discriminators in `visa_route` resolver | M3 |
| M6 frontend | family selector, level/subtype narrowing, entity wiki routing | M4, M5 |
| M7 admin+content | entity-bound type management; field schemas authored per group | M3 |

Fastest end-to-end proof (no schema needed): **`col-transportation`**
compare type bound to `wiki_city_prices_transportation` — every piece
exists today (group, city resolver, item picker). Worth shipping as the
walking skeleton before M1.

## 10. Open questions

1. **Embassy parentage.** An embassy sits at the intersection of two
   countries (UK embassy *in* Nigeria). `wiki_type_parents` to `country`
   is a start; a `host_country`/`origin_country` pair on the entity may
   be the truer model — decide whether compare needs both axes.
2. **Visa kinds as types vs. one type + kind dimension.** This spec uses
   child wiki types with discriminators (matches the mindmap). The
   lighter alternative — one `visa_route` type, kind as an item filter —
   loses per-kind field groups; rejected per the "own group" decision,
   but revisit if kind sprawl continues.
3. **`geo_fields` deprecation.** Rebinding system geo types to
   `wiki_*_overview` orphans the seeded group — migrate its fields into
   the overview groups first if any dimension is still wanted.
4. **`data_entity_type` on `wiki_pages`** — an existing column hints at
   prior polymorphic intent; confirm it isn't already load-bearing
   before adding `entity_type`/`entity_id`.
5. **Course identity.** `university_course_offerings` is
   university-scoped ("BSc Comp Engr *at* UNILAG") — matches the model
   (course nested under university). Generic course catalog rows
   (`university_course_catalog`, "BSc Computer Engineering" in the
   abstract) are a different entity — out of scope; compare only
   offerings.
