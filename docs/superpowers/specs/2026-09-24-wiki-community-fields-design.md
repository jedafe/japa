# Wiki Community Fields — Structured Community Editing for JCE Entities

**Date:** 2026-09-24
**Status:** Design
**Scope:** `internal/wiki`, `internal/customfields`, `internal/comparison`, `internal/comparison/resolver`, `internal/spaces`, `apps/main`, `apps/admin`, `migrations`, `.jeos` (§06, §30, §80, catalogs)

## 1. Problem

JCE comparisons, geo Spaces, and university Spaces all draw their structured data
from the same §06 custom-field system — yet there is no community path to keep
that data fresh. Today:

- `custom_field_values` is only written by admins, the Wikidata enrichment
  pipeline (§39), or entity owners.
- `comparison_values` are per-comparison snapshots filled by the creator or the
  AI Comparison Engine — a university's tuition changed on the source entity
  does not reach existing comparisons.
- The Destination Wiki (§80) already has the community machinery — reputation
  gating, a pending→approved revision queue, moderator review, RAG indexing —
  but it only versions TipTap prose.

The result: the data JCE renders is only as fresh as the last admin or AI pass.

## 2. Goal

Make the Wiki the community editing surface for structured entity data, so that
any entity reachable through the geo-space tree (spaces, universities, visa
routes, banks, scholarships, courses, …) can be kept current by the community,
with changes flowing into JCE comparisons automatically on approval.

Design decisions locked with the owner:

- **Full field management on the wiki page, tiered by user group.** Elevated
  groups create/edit field groups, fields, and values; the community edits
  values and proposes new fields/groups.
- **Same queue, tiered.** Structured edits ride the existing `wiki_revisions`
  pending→approved flow; elevated-tier submissions auto-approve (row kept for
  audit).
- **Suggest-an-edit anywhere.** The revision queue is entity-targeted, not
  page-bound — a shared `<SuggestEdit>` component can be dropped on any surface
  (university profile, scholarship page, JCE item card) and submit into the same
  queue. No second queue, no duplicated moderation UI.
- **Live hydration.** A comparison cell falls back to the entity's current
  `field_values` when no `comparison_values` row exists — approving a community
  edit updates every comparison containing that item on next read.
- **Entity model unchanged.** Visa routes, banks, scholarships stay in their own
  tables. The space tree is the navigation spine; `item_ref` ↔
  `(entity_type, entity_id)` is the shared addressing scheme;
  `custom_field_values` is the single source of truth.

## 3. Architecture

`wiki_revisions` becomes the **community contribution queue**. Each row carries
structured ops addressed by `(entity_type, entity_id)` — the same pair an
`item_ref` resolves to — plus optional page context recording where the edit
was submitted from.

```
member ──prose+ops──▶ wiki_revisions (pending)
admin-tier ──ops────▶ wiki_revisions (auto-approved, audit row kept)
                             │ approve (wiki.review)
                             ▼
              apply ops transactionally:
                set_value     → custom_field_values + entity.field_values
                propose_field → custom_fields (insert)
                propose_group → custom_field_groups (+assignments)
                upsert_field  → custom_fields (update; manage-tier only)
                assign_group  → custom_field_group_assignments
                             ▼
              enqueue wiki:fields_propagate
                             ▼
              comparisons containing item_ref {type,id} → cache bust
                             ▼
              matrix cells hydrate from entity.field_values (live)
```

One review queue serves every surface — page edits and entity edits are the same
row shape, filterable by `kind`.

## 4. Schema

```sql
-- Migration: wiki structured revisions
ALTER TABLE wiki_revisions
  ADD COLUMN kind                text NOT NULL DEFAULT 'content'
    CHECK (kind IN ('content','fields','mixed')),
  ADD COLUMN ops                 jsonb NOT NULL DEFAULT '[]',
  ADD COLUMN target_entity_type  text,
  ADD COLUMN target_entity_id    uuid,
  ADD COLUMN apply_error         text,  -- last ops-apply failure on a pending revision
  ALTER COLUMN wiki_page_id DROP NOT NULL;
CREATE INDEX idx_wiki_revisions_target
  ON wiki_revisions (target_entity_type, target_entity_id);

-- Migration: dimension → source field link for hydration
ALTER TABLE comparison_dimensions
  ADD COLUMN source_field_id uuid REFERENCES custom_fields(id);

-- Migration: optional child-entity binding for wiki pages (Section 8.2)
ALTER TABLE wiki_pages
  ADD COLUMN data_entity_type text;   -- e.g. 'visa_route'; NULL = page edits target the space itself
```

`wiki_page_id` nullable is the load-bearing change: it lets a fields revision
exist for an entity that has no wiki page (scholarship, bank) while page edits
keep their context. `ON DELETE CASCADE` semantics are preserved for
page-attached rows; page-less rows are unaffected.

## 5. Ops vocabulary

`ops` is a JSONB array applied in order on approval, inside one transaction:

```json
{"op":"set_value",     "entity_type":"university","entity_id":"…","field_key":"tuition_ug","value":{},"source_url":"…"}
{"op":"propose_field", "entity_type":"university","group_slug":"university_fields","field":{"key":"visa_success_rate","label":"Visa Success Rate","field_type":"percentage"}}
{"op":"propose_group", "entity_type":"university","group":{"slug":"visa_outcomes","name":"Visa Outcomes","fields":[…]},"entity_class":"space_type:university"}
{"op":"upsert_field",  "group_slug":"university_fields","field":{…}}
{"op":"assign_group",  "entity_class":"space_type:university","group_slug":"visa_outcomes"}
```

- `set_value` writes the column selected by `customfields.FieldTypeValueColumn`
  into `custom_field_values` and merges `key→value` into the entity's
  denormalized `field_values` JSONB when the entity table has that column.
- **Entity-type normalization:** space-backed ops (`university`, `country`,
  `state`, `city`, `space`) write `custom_field_values.entity_type='space'` —
  the existing convention (`enrichUniversityFields` in
  `internal/spaces/processoradapter.go`, `spaces.sql` cfv join). The revision's
  `target_entity_type` keeps the submitted type for display and propagation.
  This prevents duplicate values under both `space` and `university`.
- `propose_*` creates definition rows on approval — proposals never mutate live
  schema before review.
- `upsert_field`/`assign_group` are `wiki.fields.manage`-gated; submitted by
  lower tiers → `403 forbidden`.
- `set_value.source_url` (optional) is stored in `custom_field_values` metadata
  for provenance, matching the `comparison_data_sources` trust model (§30
  Sub-spec 4).
- `wiki.suggest.max_ops_per_revision` (default 50) bounds a submission.

## 6. Submit & review flow

1. **Submit.** `wiki.fields.edit`/`wiki.fields.propose` permission + reputation
   gate (`wiki.min_reputation_to_edit`, shared with prose). Ops are validated at
   submit: target field exists (or is being proposed), value coerces to the
   field type's value column, select/multiselect values belong to the field's
   options, and each `set_value` op passes `custom_field_permissions.can_edit`
   for the submitter's user group — any failure rejects the submission.
2. **Tiering.** Submitter holds `wiki.fields.manage` → revision is created
   `status='approved'` and ops applied immediately (audit row preserved).
   Otherwise `status='pending'`.
3. **Review.** Revision-keyed approve/reject routes (page-scoped routes remain
   as aliases). Approve runs the ops transaction;
   per-op failures roll back with an annotated error (no partial
   application — reviewers see an all-or-nothing diff), and the failure reason
   is persisted on the revision's `apply_error` column so the queue shows why a
   pending revision cannot be approved; a later successful approve clears it.
4. **Propagate.** Approved `set_value`/`upsert_field` ops enqueue
   `wiki:fields_propagate` (Asynq, default tier, idempotent on
   `{entity_type}:{entity_id}`).

## 7. API routes

```
POST   /api/v1/wiki/:space_slug/:page_slug/revisions  — extended body: { content?, ops[], summary }
                                                        kind derived: ops-only → 'fields',
                                                        content+ops → 'mixed'
POST   /api/v1/wiki/suggest                            — { entity_type, entity_id, ops[], summary }
                                                        resolves entity via JCE resolver registry;
                                                        wiki_page_id attached best-effort
GET    /api/v1/wiki/entities/:type/:id/fields          — editable schema + current values
                                                        + pending revision count for the entity
GET    /api/v1/wiki/entities/:type/:id/revisions       — entity edit history (fields revisions)
GET    /api/v1/admin/wiki/revisions?kind=&status=      — queue filter (new: kind param)
POST   /api/v1/admin/wiki/revisions/:id/approve        — NEW canonical approve (works for
POST   /api/v1/admin/wiki/revisions/:id/reject           page-less revisions; page-scoped
                                                        routes remain as aliases)
```

Approve gains the ops-application branch for `fields`/`mixed` kinds. The
revision-keyed routes exist because `wiki_page_id` is nullable — a suggested
edit on a page-less entity cannot be addressed through `:page_id`. All new
routes land in `conventions/api-routes-catalog.md`.

## 8. Entity scoping — which fields are editable

### 8.1 The entity itself

`GET /wiki/entities/:type/:id/fields` resolves the field groups assigned to the
entity via `custom_field_group_assignments`:

| entity_type | entity_class lookup |
|---|---|
| `space`, `university`, `country`, `state`, `city` | `space_type:{slug}` + `space_type:{id}` + class-wide `space_type` — `university_fields`, `space_fields` |
| `organization`, `bank` | `organization` |
| content-backed types | `content_type:{slug}` |
| table-backed JCE types (`visa_route`, `scholarship`, …) | `jce_entity:{type}` — new assignment token |

Entities with no assigned group return an empty schema; the UI then offers
"Propose a field group" — the community bootstrap path.

### 8.1a Field-permission bootstrap

`custom_field_permissions` starts effectively empty and the field-level check
is fail-closed, so without a seed the community surface would be inert.
Migration `000733` seeds: fields whose group is assigned to an entity class get
`can_view+can_edit` for every non-guest user group (assignment is the opt-in to
community editing); all other non-system fields get `can_view` only. Existing
rows are never overwritten — explicit admin denials survive — and admins narrow
grants per field/group in the admin UI. Field-level grants never bypass the
`wiki.fields.*` RBAC nodes or the reputation gate checked at submit.

### 8.2 Child entities on a space wiki page

`wiki_pages.data_entity_type` (admin-set) binds a page to a child entity type —
e.g. Canada's `visa` page → `visa_route`. The page's Data panel lists entities
via `GET /api/v1/jce/entities/{type}?geo_space_id={page.space_id}` (existing
resolver `Search` + `deepestGeoFilter`), each row editable inline through the
same ops flow. Pages with `data_entity_type = NULL` edit the space itself.

## 9. JCE live hydration

- When `CreateJCEComparison` auto-creates dimensions from a field group
  (`autoFields`), it now stores `source_field_id` on each dimension. Admins may
  set it on manual dimensions.
- `GetComparisonMatrix` cell resolution order:
  1. `comparison_values` row exists → use it (per-comparison override).
  2. Else `dimension.source_field_id` set + item `item_ref` resolvable →
     read `field_values[field.key]` from the resolved item's metadata →
     cell marked `hydrated_from: "entity_field"` for UI attribution.
  3. Else empty.
- Resolvers expose raw `field_values` in `ResolvedItem.Metadata` (DBResolver
  already reads `FieldValuesColumn` for score inputs — now also returned raw;
  space/university/content resolvers gain the same).
- `wiki:fields_propagate` finds affected comparisons via
  `SELECT content_id FROM comparison_items WHERE item_ref @> {type,id}`
  (existing GIN index) and calls `DeleteJCEComparisonCaches` for each —
  including `jc:jce:score:*`, `jc:jce:freshness:*`, `jc:jce:advisor:*` keys
  already in the catalog.

## 10. RBAC nodes

| Node | Default grant | Effect |
|---|---|---|
| `wiki.edit` | rep ≥ `wiki.min_reputation_to_edit` | prose revisions (unchanged) |
| `wiki.fields.edit` | rep ≥ `wiki.min_reputation_to_edit` | `set_value` ops |
| `wiki.fields.propose` | member | `propose_field`, `propose_group` ops |
| `wiki.fields.manage` | admin, super_admin | all ops; auto-approve own submissions |
| `wiki.review` | moderator+ | approves fields revisions (same queue) |

Per-field `custom_field_permissions.can_edit` remains authoritative for
`set_value` — a field hidden from a user group is not editable via wiki either.

## 11. Feature flags & settings

| Key | Default | Notes |
|---|---|---|
| `wiki.structured_edits` (feature flag) | `false` | requires `wiki.enabled`; gates all new routes + UI panels |
| `wiki.min_reputation_to_edit` | `100` | reused for `set_value` submissions |
| `wiki.suggest.max_ops_per_revision` | `50` | submission bound |

## 12. Frontend surfaces (apps/main, apps/admin)

- **Wiki page Data panel** — renders the bound entity's field groups; inline
  edit → ops revision. Manage tier gets group/field definition editing inline.
- **`<SuggestEdit item_ref>`** — shared component; fetches
  `GET /wiki/entities/{type}/{id}/fields`, renders the schema, submits
  `/wiki/suggest`. Mounted on university pages, entity detail pages, and JCE
  item cards.
- **Review queue (admin)** — `kind` filter; fields revisions render a
  field-level diff table (field · old → new · entity link · source_url).
- **Matrix attribution** — hydrated cells carry `hydrated_from` so the
  comparison UI can badge "community data".

## 13. Non-goals

- Community editing of `comparison_values` directly (entity fields only — the
  per-comparison override layer stays creator/AI-owned).
- Moving JCE item types into the space table; entities keep their own tables.
- Auto-approving low-reputation edits by value risk class — v1 queues all
  non-manage-tier submissions uniformly.

## 14. Catalog & spec updates (land before/with implementation, per AGENTS.md)

- `.jeos/sections/80-destination-wiki.md` — ops revision amendment
- `.jeos/sections/06-custom-field-builder-form-builder.md` — community
  governance note + `jce_entity:{type}` assignment token
- `.jeos/sections/30-generic-comparison-system.md` — `source_field_id` +
  hydration sub-spec amendment
- `conventions/api-routes-catalog.md` — new routes
- `conventions/asynq-tasks.md` — `wiki:fields_propagate`
- `conventions/system-settings-catalog.md` — `wiki.suggest.max_ops_per_revision`
- No new Valkey key patterns — invalidation reuses existing `jc:jce:*` keys.

## 15. Testing

- Service: op validation (field type → value column), per-field
  `can_edit` enforcement, tiered auto-approve, transactional apply
  (propose→assign→set_value ordering), propagation enqueue.
- Matrix: hydration fallback order, `hydrated_from` flag, cache bust on
  approved edit.
- API: `suggest` on non-space entity without a wiki page; kind derivation;
  ops bound enforcement.
