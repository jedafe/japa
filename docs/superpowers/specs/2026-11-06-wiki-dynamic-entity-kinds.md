# Wiki Entity Directory + Dynamic Entity Kinds — Design Spec

Date: 2026-11-06
Status: Draft for review (JEOS amendment to follow in §80 / §30 before code)
Refs: §80-destination-wiki, §30-generic-comparison-system (Sub-spec 6), migrations 000739, 000971, 001069, 001072

## Goal

Three scoped capabilities on `/wiki/e`:

1. `/wiki/e` — entity-kind index page ("browse the entity wiki").
2. `/wiki/e/{type_slug}` — per-kind entity listing ("All Programs", "All Courses").
3. Instance CRUD + **admin-defined entity kinds** (e.g. "AI Agents", "Websites") that auto-provision into JCE comparison types.

## Current architecture (verified)

- `/wiki/e/[type_slug]/[entity]/[page_slug]` is the only entity route — `/wiki/e` and `/wiki/e/{type}` 404 today.
- Every entity kind maps to a **hand-picked backing table** (`spaces`, `university_course_catalog`, `university_course_offerings`, …), a hand-written JCE resolver, seeded `comparison_type_definitions` + `comparison_type_field_groups` pool rows.
- `wiki_pages` binds pages to entities via `entity_type`/`entity_id` + `page_template_id` + `field_group_id`.
- Kind config lives in `wiki_entity_kinds`, `wiki_type_configs`, `wiki_type_categories`, `custom_field_groups`, `wiki_type_page_templates` — all seedable config.
- The JCE pool machinery is already kind-agnostic: dimensions carry `source_field_id`, hydration reads `field_values` from resolver metadata regardless of kind.

## Part A — Directory pages

### Backend

- `GET /wiki/entity-kinds` — `{ kinds: [{ slug, label, icon, entity_count, wiki_root }] }` from `wiki_entity_kinds` + count from each kind's source.
- `GET /wiki/entity-kinds/{slug}/entities?page&page_size&q` — paginated listing. For kinds backed by the generic store (Part C), one query. For table-backed kinds, route through a **kind source adapter** (below).

### Frontend

- `/wiki/e/+page.svelte` — kind cards (label, count, icon) → `/wiki/e/{slug}`.
- `/wiki/e/[type_slug]/+page.svelte` — entity listing (card grid, search, pagination). Compatible with the existing `[type_slug]/[entity]/[page_slug]` route (different depth).

## Part B — Instance CRUD

- `POST /wiki/entity-kinds/{slug}/entities` — create entity row + auto-create `wiki_pages` from the kind's `page_template_id`/`field_group_id` (same shape as existing entity pages).
- `PATCH/DELETE` on `entities/{id}` — soft-delete per Rule 11.
- "Create" button on `/wiki/e/{type}` gated by the kind's creation permission (`wiki.entity.create` or admin-gated).

## Part C — Dynamic entity kinds (the real change)

### Generic entity store

```
entities(
  id uuid pk,
  kind_id uuid fk → entity_kinds,
  slug text unique,
  title text,
  summary text,
  image_id uuid,
  geo_space_id uuid,          -- optional geo anchoring (offers, pricing)
  parent_entity_id uuid,      -- optional kind hierarchy (offering→program)
  created_by, deleted_at, timestamps
)
entity_kinds(id, slug unique, label, icon, entity_class, wiki_enabled, jce_enabled, config jsonb)
```

**All kind-specific data lives in `custom_field_values`** (entity_class = kind slug) — the existing field machinery (groups, categories, sections, sections-filtering) applies unchanged. This is the single design choice that makes new kinds possible: no per-kind table.

### Generic resolver

`resolver.GenericResolver{kind}` replaces hand-written resolvers for new kinds — `Search/Get/List` read `entities` + batched cfv, populating `field_values` metadata exactly as `course.go` etc. do. Existing resolvers stay; new kinds get the generic one by default.

### Kind provisioning API

`POST /admin/wiki/entity-kinds` (admin permission `wiki.entity_kind.create` — new RBAC node):

In one tx — kind row + `wiki_type_configs` + category `custom_field_groups` (from a `categories` array in the request) + `wiki_type_page_templates` (default template) +, when `jce_enabled`, `comparison_type_definitions` (slug, `item_ref_types=[kind slug]`, `field_group_id`=default group) + `comparison_type_field_groups` pool rows. The Sub-spec 6 pool seed path (`CreateComparisonType` now seeds the default row automatically) covers the pool.

### JCE wiring

- `item_ref_types` = `[kind slug]` → typed like-for-like comparisons only (consistent with existing model).
- Field groups become the pool; categories become picker groups — **zero new JCE code**, the Sub-spec 6 machinery consumes whatever cfv the resolver returns.
- Table-backed kinds (spaces, offerings) stay on their hand-written resolvers; the generic kind path covers new kinds only. A later migration may port offerings onto the generic store.

## Key decisions

| Decision | Choice | Why |
|---|---|---|
| Backing store for new kinds | Generic `entities` table + cfv | Removes the per-kind table — the blocker for "kinds on the fly" |
| Resolver | Generic, keyed by kind | New kinds can't get hand-written code; generic covers them |
| Kind creation | Admin-gated API | Entity kinds are schema-level config — user-facing creation risks taxonomy sprawl |
| JCE opt-in | `jce_enabled` flag on kind | Not every kind is comparable (e.g. navigational kinds) |
| cfv entity_class | Kind slug | Matches existing `custom_fields.entity_class` convention |

## Open questions

1. Reuse `wiki_entity_kinds` vs new `entity_kinds` table — pending schema check of `wiki_entity_kinds` columns; prefer extending it if it already holds kind metadata.
2. Do table-backed kinds (program/course/university) stay table-backed indefinitely, or migrate onto the generic store? (Spec stays dual-store for now; migration is a follow-up.)
3. Instance CRUD permission model — per-kind `wiki.entity.create` vs admin-only?
4. `geo_space_id`/`parent_entity_id` columns — confirmed needed? (offering→university links are real; check how offerings link today before finalizing.)

## Implementation order

1. Part A backend + pages (independent, ships alone)
2. `entities`/`entity_kinds` migration + generic resolver
3. Kind provisioning API + admin UI
4. Instance CRUD on `/wiki/e/{type}`
5. JEOS amendments to §80/§30 land before each step's code
