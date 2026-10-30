# Wiki University ↔ Course/Program Cross-Links — Design

**Date:** 2026-10-30
**Status:** Draft
**Builds on:** §80 (Destination Wiki), migration 000900 (entity-bound wiki types), 2026-10-20-jce-wiki-field-group-mapping-design.md

## 1. Problem

Universities and Courses/Programs are separate wiki entities with a
many-to-many relationship (`university_course_offerings` joins university
spaces to `university_course_catalog` rows). The wiki should surface that
relationship in both directions:

- A university's wiki shows a **Programs** grid — the courses it offers.
- A course wiki shows a **Universities** grid — the universities offering
  the same course.

The proposed "rollup category" mechanism does not work as-is: all three
rollup modes (`aggregate`, `directory`, `pages`) resolve descendants over
the **space hierarchy** (`space_parent_spaces ∪ spaces.parent_id` via
`ListDirectoryChildren` / `computeRollup`). Entity-bound wiki types — whose
pages bind real entity rows, not spaces — are invisible to these queries.
A `mode:"directory"` category on the university type with
`levels:["course"]` would always be empty.

## 2. Locked decisions

- **Two course levels ("both levels").** The existing `course` wiki type
  stays **offering-bound** (`university_course_offerings` — "BSc CompEngr
  *at* UNILAG", per the JCE↔wiki spec's locked course-identity decision).
  A **new** `program` wiki type binds `university_course_catalog` — the
  canonical "BSc Computer Engineering" entity.
- **Cards link both ways.** University cards on course/program pages link
  to the university wiki (`/wiki/{uni-slug}`) *and* to that university's
  course offering page (`/wiki/e/course/{offering-slug}`).
- **Catalog type slug:** `program`. `course` stays offering-bound; all
  existing `/wiki/e/course/...` URLs keep their meaning.

## 3. Domain model

```
university (space) ──< university_course_offerings >── university_course_catalog
  "UNILAG"              "BSc CompEngr AT UNILAG"         "BSc Computer Engineering"
  space-bound            wiki type: course                NEW wiki type: program
  /wiki/{slug}           /wiki/e/course/{slug}/{page}     /wiki/e/program/{slug}/{page}
```

| wiki type | binding | source table | host |
|---|---|---|---|
| `university` | space | `spaces` | — (geo hierarchy) |
| `course` | entity | `university_course_offerings` | `university_id` → one university |
| `program` (new) | entity | `university_course_catalog` | none — N universities via offerings |

`program` gets **no** `wiki_type_parents` edge. It has no single host
space (`EntityRef.ParentSpaceID` is NULL → no space breadcrumb), and
parenting it to `university` would duplicate the course list on the
university index's `entity_children` block (once as offerings, once as
programs). The existing `course → university` edge already covers the
child-listing axis.

## 4. Design — three integration points

### 4.1 University → Programs (extend `mode:"directory"` to entity children)

`computeDirectory` gains an entity branch: when a resolved descendant
type is entity-bound (`wiki_type_configs.entity_kind IS NOT NULL`), its
children resolve via the existing per-kind `ListEntityChildren` switch
instead of `ListDirectoryChildren`.

- Config on the university type's **existing `programs` category**
  (position 3, currently prose-only):
  `{"enabled": true, "mode": "directory", "levels": ["course"]}`.
- Resolution: `rollupTypeIDs` returns descendant type ids; split them by
  `entity_kind` — space-bound ids go to `ListDirectoryChildren` as today,
  entity-bound types go to `ListEntityChildren(spaceID, kind,
  discriminator)` using each type's stored `entity_kind` +
  `entity_discriminator` (same fields `ListEntityWikiTypesForParent`
  already returns for `entityChildren`).
- `HasDirectoryChildren` / the `empty` flag on
  `ListCategoriesForSpace` gets the same split so a university with zero
  offerings still hides the tab.
- `WikiDirectoryDTO` gains `entity_children` (grouped by child type
  slug), matching the shape `entityChildren` already emits:
  `{type_slug → [{id, slug, name, discriminator?, program_slug?}]}`.
  `ListCourseOfferingsForWiki` gains `c.slug AS program_slug` so each
  offering card can link the canonical program page as a secondary
  target.
- Frontend: `WikiDirectoryPanel` renders an entity-children group — the
  same card grid, but cards link `/wiki/e/{type_slug}/{slug}/overview`
  with an optional program-page sub-link. No geo_code/image (entities
  carry no media) — the card shows name + degree-type chip when the
  query exposes it.

Mixing works: a `levels:["neighborhood","course"]`-style config lists
space children and entity children as separate groups.

### 4.2 Offering page → "Also offered at" (new `mode:"related"`)

A peer relationship through `course_id`, not a hierarchy walk. New
rollup mode for entity-bound categories: `{"enabled": true, "mode":
"related"}`. Semantics are per entity kind:

- `course` (offering): sibling published offerings sharing `course_id`,
  excluding self → their universities.
- `program` (catalog): **all** published offerings for the catalog →
  their universities (no exclusion — there is no "self" university).

New sqlc queries:

- `ListUniversitiesForCourseOffering(offering_slug)` — sibling offerings
  join `spaces` on `university_id`: returns uni slug, name, geo_code,
  logo ref, and the sibling offering slug.
- `ListUniversitiesForProgram(catalog_slug)` — all published offerings'
  universities, same row shape. Dedupe by space id (one uni offering the
  same catalog course via two catalog-adjacent offerings collapses).

`GetCourseOfferingForWiki` gains `o.course_id` (needed to key the
sibling query and to link the offering page's canonical program).

`EntityPageDTO` gains `related?: {kind: "universities", items: [...]}`.
Attachment point: `GetEntityWikiPage` / `entityLazyPage` — when the
resolved category row's `rollup` parses to `mode:"related"`, attach the
kind-appropriate listing to the page DTO. Same lazy-page treatment as
space categories: a `universities` category page on a course renders
lazily with the grid; the stub materializes on first prose submission.

`validateRollupInput` accepts `"related"` alongside the existing
`"directory"`/`"pages"` mode literals. It is meaningless on space-bound
categories — compute paths ignore it there (space categories keep their
existing modes only).

### 4.3 Program page → "Universities offering this program"

Same `mode:"related"` resolver as 4.2, program branch. Program pages
render the standard entity-page chrome (breadcrumb shows no host space;
ancestors empty — correct, the program is university-agnostic).

## 5. Data changes (migration)

One migration seeds:

1. `wiki_entity_kinds`: `('program', 'university_course_catalog', 'id',
   'slug', 'Course Program')`. `university_course_catalog.slug` is
   `NOT NULL` + partially unique (`WHERE deleted_at IS NULL`) — safe for
   entity addressing.
2. `space_types`: `('program', 'Program', 'Canonical course/program
   (entity-bound wiki type)', 'book-open', ..., wiki_only=true)` — new
   fixed UUID in the seeded range.
3. `wiki_type_configs`: `(program_type_id, enabled=true, position=18,
   entity_kind='program')`. No `wiki_type_parents` rows.
4. `wiki_type_categories` for `program`: `overview` (PRIMARY group
   `wiki_program_overview`), `universities`
   (`rollup:{"enabled":true,"mode":"related"}`), `career-outcomes`.
   Groups + `custom_field_group_assignments` on entity_class `'program'`
   follow the 000901 convention.
5. `wiki_type_categories` for `course`: add `universities`
   (`rollup:{"enabled":true,"mode":"related"}`) after career-outcomes
   (position 5).
6. University `programs` category: `UPDATE wiki_type_categories SET
   rollup = '{"enabled":true,"mode":"directory","levels":["course"]}'`
   where slug='programs' on the university type.

Admin CRUD (`CreateWikiTypeCategory`/`UpdateWikiTypeCategory` +
`validateRollupInput`) needs no schema change — `rollup` is a pass-through
JSONB; only validation gains the `"related"` literal.

## 6. Backend code changes

- `internal/db/queries/wiki.sql`: `GetProgramForWiki`,
  `ListUniversitiesForCourseOffering`, `ListUniversitiesForProgram`;
  extend `GetCourseOfferingForWiki` (`o.course_id`, `c.slug AS
  program_slug`); extend `ListCourseOfferingsForWiki` (`c.slug AS
  program_slug`).
- `internal/wiki/entity.go`: `ResolveEntityRef` gains a `program`
  branch (catalog slug → EntityRef, `ParentSpaceID` NULL).
  `ListEntityChildren` gets **no** program branch — program has no
  `wiki_type_parents` edge, so `ListEntityWikiTypesForParent` never
  returns it and the branch would be dead code. `EntityPageDTO.Related`;
  attach in `attachEntityContext` when the category's rollup is
  `mode:"related"`.
- `internal/wiki/service_hierarchy.go`: `computeDirectory` entity
  branch + `entity_children` on `WikiDirectoryDTO`; `WikiRollupConfig`
  unchanged in shape (mode is free-form).
- `internal/wiki/service.go`: directory-attach paths unchanged (the
  branch lives inside `computeDirectory`).
- `internal/wiki/service_types.go`: `ListCategoriesForSpace` empty-check
  splits space vs entity children (`ListEntityChildren` existence);
  `validateRollupInput` accepts `"related"`.
- `internal/wiki/store.go` + sqlc gen for the new queries.

## 7. Frontend changes (apps/main)

- `WikiDirectoryPanel.svelte`: render `directory.entity_children`
  groups — cards link `/wiki/e/{type_slug}/{slug}/overview`; optional
  program sub-link when `program_slug` present.
- `/wiki/e/[type_slug]/[entity]/[page_slug]/+page.svelte`: render
  `page.related` as a directory-style card grid — university name →
  `/wiki/{uni-slug}` primary link, "View program at {uni}" →
  `/wiki/e/course/{offering-slug}` secondary.
- `WikiTabs` / index card surfaces get the `universities` category
  automatically once seeded (category template drives tabs; no
  frontend special-casing beyond the related-grid renderer).
- The bottom "Related entities" list on `/wiki/{space}` stays — it
  still serves entity types without a dedicated category (visa routes,
  banks, embassies on geo pages; scholarships on universities).
- i18n keys: `wiki.universities_offering`, `wiki.also_offered_at`,
  `wiki.view_program_at`, `wiki.related_*` labels.

## 8. Error handling & edge cases

- Catalog row with zero published offerings → program `universities`
  grid absent (`related: nil`); the lazy category page still renders
  prose/fields.
- Offering whose catalog row is unpublished/deleted → offering page
  itself already 404s (join + `is_published`/`deleted_at` guard in
  `GetCourseOfferingForWiki`).
- Resolver failures (store errors) degrade to empty payload — the page
  renders without the grid, never 500s (mirrors `computeDirectory`'s
  fail-open behavior).
- `mode:"related"` on a space-bound category or an entity kind with no
  resolver → nil payload; validation accepts the literal but compute
  ignores it (documented).
- Cap: universities-per-program capped at `defaultDirectoryLimit` (200)
  with `total` reported for truncation copy — a mega-program listing
  every university stays bounded.

## 9. Testing

- `service_hierarchy_test.go`: directory-mode category with an
  entity-bound level lists offering children; mixed space+entity
  levels; empty gating.
- `entity_test.go` (or new): `mode:"related"` on `course` returns
  sibling unis minus self; on `program` returns all offering unis;
  dedupe on double-offering.
- `service_types_test.go`: `validateRollupInput` accepts `"related"`,
  still rejects garbage modes.
- Frontend: entity-children group rendering in `WikiDirectoryPanel`;
  related grid on the entity page route.

## 10. Non-goals

- Rebinding `course` to the catalog, or merging offering/catalog pages.
- Catalog-level field population (program cfv values start empty —
  community field ops fill them over time).
- JCE resolver changes — `course` compare stays offering-scoped; a
  `program` resolver is a follow-up if wanted.
- Scholarship/bank/embassy `related` categories — the mode generalizes
  to them but only course↔program↔university ships now.
- Mobile app surfaces (follow-up if wiki entity pages exist there).

## 11. Open questions (resolved)

- ~~Catalog vs offering identity~~ → both; offering stays, `program`
  type added for catalog.
- ~~Card link target~~ → university wiki + that uni's offering page.
- ~~Type slug~~ → `program`.
