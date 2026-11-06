# JCE Multi-Category Dimension Pools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let comparison creators pick which wiki categories (and individual fields within them) feed a JCE comparison's dimensions, instead of being locked to the type's single bound field group.

**Architecture:** A new join table `comparison_type_field_groups` defines each comparison type's allowed field pool, seeded via an explicit comparison-type-slug → wiki space-type-slug map (NOT derived from the bound group — geo types are bound to the shared non-category `geo_fields` group, and the `wiki_city_prices_*` groups bound to COL subtypes are not `wiki_type_categories` rows either). The create contract gains `field_ids`/`group_ids`; selected fields materialize as dimensions with `source_field_id`, so existing cfv hydration fills cells unchanged. A new read endpoint exposes the pool to the `/compare` picker.

**Tech Stack:** Go (Chi, sqlc/pgx), PostgreSQL, SvelteKit 2 + Svelte 5 (apps/main), Bun.

## Global Constraints

- JEOS `.jeos/sections/30-generic-comparison-system.md` is source of truth — spec amendment lands **before** code (Gate 1).
- Migration numbering continues from `001071`; always an `.up.sql`/`.down.sql` pair; never edit an applied migration.
- sqlc: edit `internal/db/queries/*.sql` only; regenerate with `make sqlc-gen`. Never hand-edit `internal/db/gen/`.
- Commit to `main` only. No AI-attribution strings anywhere (Rule 17).
- Bun for all frontend tooling (Rule 18).
- Back-compat: empty `field_ids`/`group_ids`/`dimensions` must reproduce today's auto-Overview behavior, including `config.sections` filtering.

## Verified facts this plan depends on

- `CreateJCEComparison` auto-creates dims only from `typeDef.FieldGroupID` (`jce_service.go:~365`); `input.Dimensions []string` produces label-only dims with `SourceFieldID: pgtype.UUID{}` — no hydration.
- Hydration keys on `dim.SourceFieldKey` → `resolved.Metadata["field_values"][key]` (`service.go:hydrateMatrixCell`); resolvers merge **all** entity cfv via `customfields.EntityFieldValues` — group-agnostic. No matrix changes needed.
- `fieldToDimensionType(fieldType string, options []byte) string` exists at `service.go:385`; returns `""` for non-comparable composites (skip rule, reuse it).
- `dedupeComparisonDimensions` keys on `position:label` — same-label fields at different positions survive; duplicate column labels need a disambiguation suffix (handled in Task 4).
- `CreateJCEComparisonFromToken` delegates to `CreateJCEComparison` — new input fields flow through untouched.
- `comparison_content_metadata` has **no** `type_id`; a comparison's type resolves from `Items[0].TypeID` (same pattern as `PromoteJCEComparison:852`).
- Store wraps `*gen.Queries` as `s.read`/`s.write` (`store.go:13`); tx work uses `db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries)…)`.
- Current bindings: `countries`/`states`/`cities`→`geo_fields`; `universities`→`wiki_university_overview`; `courses`→`wiki_course_overview`; `scholarships`→`wiki_scholarship_overview`; `visa-types`→`wiki_visa_route_overview`; `banks`→`wiki_bank_overview`; `cost_of_living`→`wiki_city_cost_of_living`; col-*→`wiki_city_prices_*`; `mentors`→`mentor_fields`; `embassies`/`neighborhoods`/`job-listings`/`housing`/`remittance`/`language-tests`/`travel-routes`→their seed groups. Only some bound groups are `wiki_type_categories` rows — hence the explicit slug map.

---

### Task 1: JEOS spec amendment — Sub-spec 6

**Files:**
- Modify: `.jeos/sections/30-generic-comparison-system.md` (append after Sub-spec 5 block)
- Modify: `.jeos/conventions/api-routes-catalog.md` (JCE block, ~line 620)
- Modify: `docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md` (§8 "No group picker ever" line — supersede)

- [ ] **Step 1: Append the Sub-spec 6 amendment to §30**

````markdown
## JCE Sub-spec 6 Amendment (2026-11-06) — Multi-Group Dimension Pools

A comparison type's dimension pool is no longer a single `field_group_id`.
`comparison_type_field_groups` lists every field group a type may draw
dimensions from; creators pick whole wiki categories and/or individual fields
at creation time.

### Schema

```sql
CREATE TABLE comparison_type_field_groups (
    type_id    uuid NOT NULL REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    group_id   uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    category   text,        -- wiki_type_categories.slug for UI grouping (null for non-category groups)
    label      text,        -- display label; falls back to category label / group name
    position   int  NOT NULL DEFAULT 0,
    is_default boolean NOT NULL DEFAULT false,  -- the group's legacy role: auto-dims source
    PRIMARY KEY (type_id, group_id)
);
```

### Pool seeding rule

Pool membership is **explicit**, seeded from a comparison-type-slug →
space_type-slug map — derivation via the bound group's `wiki_type_categories`
row is rejected because several types bind non-category groups
(`countries`/`states`/`cities`→`geo_fields`, COL subtypes→`wiki_city_prices_*`
which are not category rows) and entity-kind groups share `entity_class`
across kinds (all visa kinds → `visa_route`). Non-wiki types (mentors,
remittance, …) get pool = bound group only.

### Create contract

`POST /jce/comparisons` and `POST /jce/comparisons/from-token` accept two new
optional arrays:

- `group_ids`: whole pool groups — every comparable field becomes a dimension
- `field_ids`: individual `custom_fields.id` picks across pool groups

`dimensions`, `field_ids`, and `group_ids` are mutually exclusive (422 on
combinations). When all three are empty, behavior is unchanged: auto-dims
from `field_group_id` honoring `config.sections`. `config.sections` applies
only to the legacy default-group expansion; explicitly selected groups yield
all comparable fields.

`POST /jce/comparisons/{id}/dimensions` accepts optional `field_id` to bind a
pool field post-create (label, dimension_type, source_field_id come from the
field def; pool validated against the comparison's resolved type — from
`items[0].type_id`, same as promote).

`PATCH /jce/comparison-types/{id}` accepts `field_group_ids` (full pool
replace; the bound `field_group_id` is always re-inserted `is_default`).

### New route

`GET /jce/comparison-types/{id}/fields` — pool groups with fields for the
picker (OptionalAuth, same feature gates). Each field carries `comparable`
so the UI can disable non-comparable composites.

### Known limitations

- **Field-key collision**: hydration keys on `field.key` over the entity's
  whole cfv bucket; `custom_fields.key` is unique per group only. Two pool
  groups reusing a key (e.g. `provider`) hydrate both dims from the same
  value. Accepted for now.
- **Duplicate labels**: fields from different groups may share a label;
  dimension labels are suffixed with the category label on collision.
````

- [ ] **Step 2: Update the api-routes catalog**

In the JCE block of `.jeos/conventions/api-routes-catalog.md`, add after the
`GET /jce/comparison-types/{slug}` line:

```
GET    /jce/comparison-types/{id}/fields     dimension pool: groups + fields (Sub-spec 6)
```

Extend the `POST /jce/comparisons` and `POST /jce/comparisons/from-token`
annotations to mention `field_ids`/`group_ids`.

- [ ] **Step 3: Supersede the design doc's "no group picker" line**

In `docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md`
§8, replace `- No group picker ever — the group is always implied by (type, level, subtype).` with:

```markdown
- ~~No group picker ever~~ **Superseded by §30 Sub-spec 6 (2026-11):** the
  type's pool spans its wiki type's category groups; a group/field picker
  lets creators choose categories and fields at creation.
```

- [ ] **Step 4: Commit**

```bash
git add .jeos/sections/30-generic-comparison-system.md .jeos/conventions/api-routes-catalog.md docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
git commit -m "spec: JCE sub-spec 6 — multi-group dimension pools"
```

---

### Task 2: Migration pair `001072_jce_dimension_pools`

**Files:**
- Create: `migrations/001072_jce_dimension_pools.up.sql`
- Create: `migrations/001072_jce_dimension_pools.down.sql`

Check `ls migrations | grep -v archive | sort | tail -3` first — renumber if `001072` is taken.

- [ ] **Step 1: Write the up migration**

```sql
-- 001072_jce_dimension_pools.up.sql
-- JCE Sub-spec 6: multi-group dimension pools.
--
-- Seeding: bound group always joins the pool as is_default. Wiki-bound types
-- additionally get every category group of their wiki type via an explicit
-- comparison-type-slug → space_type-slug map (the bound group alone cannot
-- locate the wiki type: geo types bind shared non-category 'geo_fields',
-- COL subtypes bind non-category 'wiki_city_prices_*' groups).

BEGIN;

CREATE TABLE comparison_type_field_groups (
    type_id    uuid NOT NULL REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    group_id   uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    category   text,
    label      text,
    position   int  NOT NULL DEFAULT 0,
    is_default boolean NOT NULL DEFAULT false,
    PRIMARY KEY (type_id, group_id)
);
CREATE INDEX idx_comparison_type_field_groups_group
    ON comparison_type_field_groups (group_id);

-- 1. Wiki-bound types: every category group of the mapped wiki type.
--    is_default marks whichever row is the type's current bound group.
INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, wtc.field_group_id, wtc.slug, wtc.label, wtc.position,
       (wtc.field_group_id = td.field_group_id)
FROM comparison_type_definitions td
JOIN (VALUES
    ('countries',        'country'),
    ('states',           'state'),
    ('cities',           'city'),
    ('neighborhoods',    'neighborhood'),
    ('universities',     'education'),
    ('courses',          'course'),
    ('scholarships',     'scholarship'),
    ('banks',            'bank'),
    ('embassies',        'embassy'),
    ('visa-types',       'visa_route'),
    ('visa-routes',      'visa_route'),
    ('student-visas',    'student_visa'),
    ('work-visas',       'work_visa'),
    ('family-visas',     'family_visa'),
    ('tourist-visas',    'tourist_visa'),
    ('business-visas',   'business_visa'),
    ('pr-pathways',      'pr_pathway'),
    ('cost_of_living',   'city'),
    ('col-transportation','city'),
    ('col-restaurants',  'city'),
    ('col-food-groceries','city'),
    ('col-utilities',    'city'),
    ('col-lifestyle',    'city')
) AS m(type_slug, space_slug) ON m.type_slug = td.slug
JOIN space_types st ON st.slug = m.space_slug
JOIN wiki_type_categories wtc ON wtc.space_type_id = st.id
ON CONFLICT DO NOTHING;

-- 2. Bound group as pool member for any type not covered above
--    (non-wiki types: mentors, remittance, job-listings, …) and for bound
--    groups outside the category set (geo_fields on the geo types).
INSERT INTO comparison_type_field_groups (type_id, group_id, label, position, is_default)
SELECT td.id, td.field_group_id, g.name, -1, true
FROM comparison_type_definitions td
JOIN custom_field_groups g ON g.id = td.field_group_id
WHERE td.field_group_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM comparison_type_field_groups p
      WHERE p.type_id = td.id AND p.group_id = td.field_group_id
  )
ON CONFLICT DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 001072_jce_dimension_pools.down.sql
BEGIN;
DROP TABLE IF EXISTS comparison_type_field_groups;
COMMIT;
```

- [ ] **Step 3: Apply + verify against dev DB**

```bash
make migrate
-- sanity: 'cities' → geo_fields(default) + every city category group;
-- 'col-transportation' → wiki_city_prices_transportation + all city categories;
-- 'mentors' → mentor_fields only; 'embassies' → all 5 embassy categories
SELECT td.slug, count(*) FROM comparison_type_field_groups p
JOIN comparison_type_definitions td ON td.id = p.type_id GROUP BY 1 ORDER BY 1;
```

- [ ] **Step 4: Commit**

```bash
git add migrations/001072_jce_dimension_pools.*
git commit -m "db: comparison_type_field_groups pool table + wiki category seed"
```

---

### Task 3: sqlc queries + regen

**Files:**
- Modify: `internal/db/queries/comparison.sql` (append)
- Regen: `internal/db/gen/` via `make sqlc-gen`

- [ ] **Step 1: Append queries**

```sql
-- name: ListComparisonTypeFieldGroups :many
SELECT ctfg.group_id, ctfg.category, ctfg.label, ctfg.position, ctfg.is_default,
       g.slug AS group_slug, g.name AS group_name
FROM comparison_type_field_groups ctfg
JOIN custom_field_groups g ON g.id = ctfg.group_id
WHERE ctfg.type_id = @type_id
ORDER BY ctfg.is_default DESC, ctfg.position ASC, g.name ASC;

-- name: ListComparisonTypePoolFields :many
SELECT f.id, f.group_id, f.label, f.key, f.field_type, f.options, f.position, f.section,
       ctfg.is_default AS group_is_default, ctfg.category AS group_category,
       ctfg.label AS group_label
FROM comparison_type_field_groups ctfg
JOIN custom_fields f ON f.group_id = ctfg.group_id
WHERE ctfg.type_id = @type_id
ORDER BY ctfg.is_default DESC, ctfg.position ASC, f.position ASC, f.created_at ASC;

-- name: ListPoolFieldsByGroupIDs :many
SELECT f.id, f.group_id, f.label, f.key, f.field_type, f.options, f.position, f.section
FROM custom_fields f
JOIN comparison_type_field_groups ctfg ON ctfg.group_id = f.group_id
WHERE ctfg.type_id = @type_id
  AND ctfg.group_id = ANY(@group_ids::uuid[])
ORDER BY ctfg.position ASC, f.position ASC;

-- name: CountPoolFieldsByIDs :one
SELECT count(*) FROM custom_fields f
JOIN comparison_type_field_groups ctfg ON ctfg.group_id = f.group_id
WHERE ctfg.type_id = @type_id
  AND f.id = ANY(@field_ids::uuid[]);

-- name: ListCustomFieldsByIDs :many
SELECT id, group_id, label, key, field_type, options, is_required, is_system,
       show_in_frontend, position, section, created_at
FROM custom_fields
WHERE id = ANY(@ids::uuid[]);

-- name: DeleteComparisonTypeFieldGroups :exec
DELETE FROM comparison_type_field_groups WHERE type_id = @type_id;

-- name: CreateComparisonTypeFieldGroup :exec
INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
VALUES (@type_id, @group_id, @category, @label, @position, @is_default);
```

- [ ] **Step 2: Regenerate**

```bash
make sqlc-gen && go build ./internal/db/gen/
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/comparison.sql internal/db/gen/
git commit -m "db: sqlc queries for comparison type field pools"
```

---

### Task 4: Store + service — pool resolution, create inputs, pool listing

**Files:**
- Modify: `internal/comparison/store.go` — thin wrappers on `s.read`/`s.write` matching existing style
- Modify: `internal/comparison/service.go` — `dimensionField` type, pool helpers, `ListComparisonTypeFieldPool`, `CreateComparisonDimension` field binding, `UpdateComparisonType` pool replace
- Modify: `internal/comparison/jce_service.go` — `CreateJCEComparisonInput` (+`FieldIDs`, `GroupIDs`), pool resolution replacing the `autoFields` block (~line 365)

**Interfaces:**
- Produces:
  - `type dimensionField struct { ID, GroupID pgtype.UUID; Label, Key, FieldType string; Options []byte; Section *string }`
  - `type PoolField struct { ID, Key, Label, FieldType string; Section *string; Comparable bool }`
  - `type PoolGroup struct { GroupID, Slug, Label, Category string; IsDefault bool; Fields []PoolField }`
  - `Service.ListComparisonTypeFieldPool(ctx, typeID pgtype.UUID) ([]PoolGroup, error)`
  - `CreateJCEComparisonInput.FieldIDs, .GroupIDs []string`
  - `CreateComparisonDimensionInput.FieldID string`
  - `UpdateComparisonTypeInput.FieldGroupIDs *[]string`

- [ ] **Step 1: Store wrappers** (mirror existing `getComparisonContentMetadata` style — wrap `s.read`/`s.write`, wrap errors)

```go
func (s *Store) listComparisonTypeFieldGroups(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypeFieldGroupsRow, error)
func (s *Store) listComparisonTypePoolFields(ctx context.Context, typeID pgtype.UUID) ([]gen.ListComparisonTypePoolFieldsRow, error)
func (s *Store) listPoolFieldsByGroupIDs(ctx context.Context, typeID pgtype.UUID, ids []pgtype.UUID) ([]gen.ListPoolFieldsByGroupIDsRow, error)
func (s *Store) countPoolFieldsByIDs(ctx context.Context, typeID pgtype.UUID, ids []pgtype.UUID) (int64, error)
func (s *Store) listCustomFieldsByIDs(ctx context.Context, ids []pgtype.UUID) ([]gen.ListCustomFieldsByIDsRow, error)
```

- [ ] **Step 2: `dimensionField` + conversion + UUID parsing helper** (service.go — `resolver.ParseUUID` parses one id; wrap it)

```go
type dimensionField struct {
    ID, GroupID       pgtype.UUID
    Label, Key        string
    FieldType         string
    Options           []byte
    Section           *string
}

func dimensionFieldsFromGroup(rows []gen.ListCustomFieldsByGroupRow) []dimensionField { /* field-by-field copy */ }

func parseUUIDList(ids []string) ([]pgtype.UUID, error) {
    out := make([]pgtype.UUID, 0, len(ids))
    for _, s := range ids {
        id, err := resolver.ParseUUID(s)
        if err != nil {
            return nil, err
        }
        out = append(out, id)
    }
    return out, nil
}
```

- [ ] **Step 3: Pool resolvers** (service.go)

```go
// poolFieldsByIDs resolves explicit field picks; every id must be a field in
// the type's pool (§30 Sub-spec 6). Order follows the caller's array.
func (s *Service) poolFieldsByIDs(ctx context.Context, typeID pgtype.UUID, ids []string) ([]dimensionField, error) {
    parsed, err := parseUUIDList(ids)
    if err != nil {
        return nil, fmt.Errorf("%w: invalid field_ids", errpkg.ErrInvalid)
    }
    n, err := s.store.countPoolFieldsByIDs(ctx, typeID, parsed)
    if err != nil {
        return nil, fmt.Errorf("counting pool fields: %w", err)
    }
    if int(n) != len(parsed) {
        return nil, fmt.Errorf("%w: field_ids contains fields outside this type's dimension pool", errpkg.ErrInvalid)
    }
    rows, err := s.store.listCustomFieldsByIDs(ctx, parsed)
    if err != nil {
        return nil, fmt.Errorf("listing fields: %w", err)
    }
    byID := make(map[[16]byte]gen.ListCustomFieldsByIDsRow, len(rows))
    for _, r := range rows {
        byID[r.ID.Bytes] = r
    }
    out := make([]dimensionField, 0, len(parsed))
    for _, id := range parsed {
        if r, ok := byID[id.Bytes]; ok {
            out = append(out, dimensionField{ID: r.ID, GroupID: r.GroupID, Label: r.Label, Key: r.Key, FieldType: r.FieldType, Options: r.Options, Section: r.Section})
        }
    }
    return disambiguateFieldLabels(out, nil), nil
}

// poolFieldsByGroups expands requested groups after validating each is a pool
// member, then fills dims in pool order. Labels collide across groups —
// suffix the group/category label on duplicates.
func (s *Service) poolFieldsByGroups(ctx context.Context, typeID pgtype.UUID, ids []string) ([]dimensionField, error) {
    parsed, err := parseUUIDList(ids)
    if err != nil {
        return nil, fmt.Errorf("%w: invalid group_ids", errpkg.ErrInvalid)
    }
    pool, err := s.store.listComparisonTypeFieldGroups(ctx, typeID)
    if err != nil {
        return nil, fmt.Errorf("listing pool groups: %w", err)
    }
    inPool := make(map[[16]byte]string, len(pool))
    for _, g := range pool {
        label := g.GroupName
        if g.Label != nil && *g.Label != "" {
            label = *g.Label
        }
        inPool[g.GroupID.Bytes] = label
    }
    for _, id := range parsed {
        if _, ok := inPool[id.Bytes]; !ok {
            return nil, fmt.Errorf("%w: group_ids contains groups outside this type's dimension pool", errpkg.ErrInvalid)
        }
    }
    rows, err := s.store.listPoolFieldsByGroupIDs(ctx, typeID, parsed)
    if err != nil {
        return nil, fmt.Errorf("listing pool group fields: %w", err)
    }
    out := make([]dimensionField, 0, len(rows))
    for _, r := range rows {
        out = append(out, dimensionField{ID: r.ID, GroupID: r.GroupID, Label: r.Label, Key: r.Key, FieldType: r.FieldType, Options: r.Options, Section: r.Section})
    }
    return disambiguateFieldLabels(out, inPool), nil
}

// disambiguateFieldLabels suffixes " (group label)" when a label repeats.
func disambiguateFieldLabels(fields []dimensionField, groupLabels map[[16]byte]string) []dimensionField {
    counts := map[string]int{}
    for _, f := range fields {
        counts[f.Label]++
    }
    for i, f := range fields {
        if counts[f.Label] > 1 {
            if gl, ok := groupLabels[f.GroupID.Bytes]; ok && gl != "" {
                fields[i].Label = f.Label + " (" + gl + ")"
            }
        }
    }
    return fields
}
```

- [ ] **Step 4: Rewire `CreateJCEComparison` auto-dims** (jce_service.go ~365)

```go
var autoFields []dimensionField

switch {
case len(input.Dimensions) > 0 && (len(input.FieldIDs) > 0 || len(input.GroupIDs) > 0):
    return JCEComparisonResult{}, fmt.Errorf("%w: dimensions is mutually exclusive with field_ids/group_ids", errpkg.ErrInvalid)
case len(input.FieldIDs) > 0 && len(input.GroupIDs) > 0:
    return JCEComparisonResult{}, fmt.Errorf("%w: field_ids and group_ids are mutually exclusive", errpkg.ErrInvalid)
case len(input.FieldIDs) > 0:
    if !input.TypeID.Valid {
        return JCEComparisonResult{}, fmt.Errorf("%w: field_ids requires a comparison type", errpkg.ErrInvalid)
    }
    if autoFields, err = s.poolFieldsByIDs(ctx, input.TypeID, input.FieldIDs); err != nil {
        return JCEComparisonResult{}, err
    }
case len(input.GroupIDs) > 0:
    if !input.TypeID.Valid {
        return JCEComparisonResult{}, fmt.Errorf("%w: group_ids requires a comparison type", errpkg.ErrInvalid)
    }
    if autoFields, err = s.poolFieldsByGroups(ctx, input.TypeID, input.GroupIDs); err != nil {
        return JCEComparisonResult{}, err
    }
case len(input.Dimensions) == 0 && typeDef != nil && typeDef.FieldGroupID.Valid:
    fields, ferr := s.cfSvc.ListFieldsByGroup(ctx, typeDef.FieldGroupID)
    if ferr != nil {
        return JCEComparisonResult{}, fmt.Errorf("listing field-group fields: %w", ferr)
    }
    autoFields = dimensionFieldsFromGroup(filterFieldsByConfigSections(fields, typeDef.Config))
}
```

The existing dim-creation loop (`f.Key == ""` skip, `fieldToDimensionType`, `SourceFieldID: f.ID`) works on `dimensionField` unchanged — only the field accesses swap names.

- [ ] **Step 5: `ListComparisonTypeFieldPool`** (service.go)

```go
type PoolField struct {
    ID        string  `json:"id"`
    Key       string  `json:"key"`
    Label     string  `json:"label"`
    FieldType string  `json:"field_type"`
    Section   *string `json:"section,omitempty"`
    Comparable bool   `json:"comparable"`
}
type PoolGroup struct {
    GroupID   string      `json:"group_id"`
    Slug      string      `json:"slug"`
    Label     string      `json:"label"`
    Category  *string     `json:"category,omitempty"`
    IsDefault bool        `json:"is_default"`
    Fields    []PoolField `json:"fields"`
}

// ListComparisonTypeFieldPool groups the type's dimension pool for the picker.
// Comparable = fieldToDimensionType != "" — non-comparable composites are
// returned flagged so the UI can disable rather than hide them.
func (s *Service) ListComparisonTypeFieldPool(ctx context.Context, typeID pgtype.UUID) ([]PoolGroup, error) {
    groups, err := s.store.listComparisonTypeFieldGroups(ctx, typeID)
    // …bucket ListComparisonTypePoolFields rows per group_id;
    // label fallback: ctfg.label → category label → group_name.
}
```

- [ ] **Step 6: `CreateComparisonDimension` `field_id` binding** (service.go ~1260)

Add `FieldID string` to the input. When set: fetch the field via
`listCustomFieldsByIDs`, resolve the comparison's type from
`s.store.listComparisonItemsByContent(contentID)` first non-null `type_id`
(Promote pattern), `countPoolFieldsByIDs` to validate membership, then
override `Label`/`DimensionType`/`SourceFieldID` from the field def
(`fieldToDimensionType`; `""` → `ErrInvalid "field is not comparable"`).

- [ ] **Step 7: `UpdateComparisonType` pool replace + `CreateComparisonType` default row**

- `UpdateComparisonTypeInput` += `FieldGroupIDs *[]string`. When non-nil:
  `db.WithTx` → `DeleteComparisonTypeFieldGroups` → re-insert bound
  `field_group_id` `is_default` + each listed group (order → position).
- `CreateComparisonType`: after insert, write one pool row
  `(new_type_id, input.FieldGroupID, is_default=true)` when a group was bound —
  keeps new types consistent with the migration seed.

- [ ] **Step 8: Build + vet + commit**

```bash
go build ./... && go vet ./...
git add internal/comparison/
git commit -m "jce: multi-group dimension pools in create contract (sub-spec 6)"
```

---

### Task 5: Handlers + routes

**Files:**
- Modify: `internal/comparison/jce_handlers.go` — create body (~line 195), from-token body (~line 778)
- Modify: `internal/comparison/handlers.go` — `CreateComparisonDimension` body (~line 1002), `UpdateComparisonType` body (~line 469), new `GetComparisonTypeFields`
- Modify: `internal/server/server.go` — route registration (~line 5077)

- [ ] **Step 1: Request-body additions**

`jceCreateBody` + the from-token struct + `CreateComparisonDimension` input:

```go
FieldIDs []string `json:"field_ids"`
GroupIDs []string `json:"group_ids"`
// dimension create only:
FieldID  string   `json:"field_id"`
```

`UpdateComparisonType` input: `FieldGroupIDs *[]string `json:"field_group_ids"``
(pointer distinguishes absent from empty: nil = untouched, empty = reset pool
to bound group only).

- [ ] **Step 2: `GetComparisonTypeFields` handler** (handlers.go)

```go
// GetComparisonTypeFields serves GET /api/v1/jce/comparison-types/{id}/fields —
// the Sub-spec 6 dimension pool for the /compare picker.
func (h *Handlers) GetComparisonTypeFields(w http.ResponseWriter, r *http.Request) {
    id, err := parseUUIDParam(r, "id")
    if err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid type ID")
        return
    }
    pool, err := h.Service.ListComparisonTypeFieldPool(r.Context(), id)
    if err != nil {
        handleServiceError(w, err)
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"groups": pool}})
}
```

- [ ] **Step 3: Register the route** (server.go, beside the `{slug}` GET — extra path segment, no conflict)

```go
r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/jce/comparison-types/{id}/fields", comparisonHandlers.GetComparisonTypeFields)
```

- [ ] **Step 4: Build + commit**

```bash
go build ./... && go vet ./...
git add internal/comparison/jce_handlers.go internal/comparison/handlers.go internal/server/server.go
git commit -m "jce: field-pool endpoint + pool-aware create routes"
```

---

### Task 6: Backend tests

**Files:**
- Create: `internal/comparison/dimension_pool_test.go` (follow `jce_test.go`/`matrix_hydration_test.go` testdb fixture style)
- Extend: `internal/comparison/jce_test.go` (from-token path if fixtures allow)

- [ ] **Step 1: Group expansion test** — seed type + 2 pool groups + fields; `GroupIDs=[both]` → dims created for every comparable field in pool order, each with `source_field_id` set.
- [ ] **Step 2: `field_ids` validation test** — field outside pool → `ErrInvalid` ("outside this type's dimension pool"); inside pool → typed dim with `source_field_id`.
- [ ] **Step 3: Mutual exclusion tests** — `dimensions`+`field_ids`, `field_ids`+`group_ids` → `ErrInvalid`.
- [ ] **Step 4: Back-compat test** — all empty → bound-group dims only; `config.sections` still filters.
- [ ] **Step 5: Label disambiguation test** — two groups each with a `Provider` field → dims labeled `Provider (Overview)` / `Provider (Accounts & Fees)`.
- [ ] **Step 6: Pool endpoint test** — `ListComparisonTypeFieldPool` ordering, `is_default` flag, non-comparable fields present with `comparable: false`.
- [ ] **Step 7: Post-create `field_id` test** — add dim with `field_id` → inherits label/type/source_field_id; out-of-pool → `ErrInvalid`.

- [ ] **Step 8: Run + commit**

```bash
go test ./internal/comparison/ -v
git add internal/comparison/dimension_pool_test.go internal/comparison/jce_test.go
git commit -m "jce: dimension pool tests"
```

---

### Task 7: Frontend — category + field picker on /compare

**Files:**
- Modify: `apps/main/src/lib/types/jce.ts` — `JCEPoolField`, `JCEFieldPoolGroup`
- Modify: `apps/main/src/lib/api/jce.ts` — pool fetch helper + `field_ids` on the create call
- Modify: `apps/main/src/routes/(app)/compare/+page.server.ts` — fetch pool on type select, forward `field_ids` on create
- Create: `apps/main/src/lib/components/compare/FieldPoolPicker.svelte`
- Modify: `apps/main/src/lib/i18n/jce_en.json`, `jce_fr.json`

Direct-create surfaces (`universities`, `jobs`, `org`, `tools/*` `+page.server.ts`)
POST `/jce/comparisons` — they get `field_ids` support free; wiring pickers
there is follow-up, not this task.

- [ ] **Step 1: Types** (jce.ts)

```ts
export interface JCEPoolField {
    id: string;
    key: string;
    label: string;
    field_type: string;
    section?: string | null;
    /** false → render disabled (non-comparable composite) */
    comparable: boolean;
}

export interface JCEFieldPoolGroup {
    group_id: string;
    slug: string;
    label: string;
    category?: string | null;
    is_default: boolean;
    fields: JCEPoolField[];
}
```

- [ ] **Step 2: Fetch + forward**

On `selectedType` change in `+page.server.ts`:
`apiGet<{groups: JCEFieldPoolGroup[]}>(ctx, `/jce/comparison-types/${selectedType.id}/fields`)`
— graceful empty on error (picker hidden, legacy auto-dims still apply).
On create (the `/jce/comparisons/from-token` call and any direct
`/jce/comparisons` POST on this page): add `field_ids: [...selectedIds]`
to the body only when the selection differs from "all comparable fields of
the default group" — keeps payloads minimal and back-compat.

- [ ] **Step 3: `FieldPoolPicker.svelte`** (Svelte 5 runes)

- Props: `groups: JCEFieldPoolGroup[]`; bind `selected = $state(new Set<string>())`.
- Init: all comparable fields of `is_default` groups selected.
- Per group: header checkbox (checked/indeterminate from `$derived` selected
  count) + label/category; expandable per-field list; `comparable: false`
  fields rendered disabled with a hint.
- "All categories" toggle at top.
- i18n keys: `jce.compare.fields.title`, `jce.compare.fields.all`,
  `jce.compare.fields.non_comparable` — add to both jce_en.json and jce_fr.json.

- [ ] **Step 4: Wire into the wizard**

Render after type selection (pool depends only on the type, not items).
`selected` ids feed the create body.

- [ ] **Step 5: Check + build + commit**

```bash
cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json && bun run build
git add apps/main/
git commit -m "compare: category + field pool picker (jce sub-spec 6)"
```

---

### Task 8: Final verification + catalogs

- [ ] **Step 1:** `go build ./... && go vet ./...` clean; `go test ./internal/comparison/` green.
- [ ] **Step 2:** `cd apps/main && bun run build` green.
- [ ] **Step 3: Manual smoke (dev stack) — acceptance scenario:** on `/compare`, select **Courses**, pick 2+ course offerings, select the Overview + Career Outcomes + Entry Requirements categories → the matrix shows dims from all three groups with cells hydrated from each course's cfv (`hydrated_from: entity_field`). Repeat with Overview only and with a 2-category subset to confirm selection is honored.
- [ ] **Step 4: Catalog sweep** — api-routes-catalog has the new route + request fields; **no** new Valkey keys, system settings, Asynq tasks, or RBAC nodes were introduced (existing `feature.comparison`+`feature.jce` gates cover all new routes).
