# Wiki University ↔ Course/Program Cross-Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bidirectional university↔program linking in the destination wiki — a new `program` entity-bound wiki type over `university_course_catalog`, a Programs grid on university wikis, and Universities grids on course/program wiki pages.

**Architecture:** Extend `mode:"directory"` rollups to entity-bound descendant types (space→children axis), add a new `mode:"related"` peer resolver for entity-bound category pages (course→sibling unis, program→all offering unis), and seed a `program` wiki type binding `university_course_catalog`.

**Tech Stack:** Go (chi, sqlc/pgx v5), PostgreSQL, golang-migrate, SvelteKit 2 + Svelte 5 + Tailwind v4.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-10-30-wiki-university-course-program-links-design.md` — read it first.
- sqlc codegen: `make sqlc-gen` (runs `sqlc generate`); gen output lives in `internal/db/gen/`. Never hand-edit gen files.
- Migrations: paired `NNNNNN_name.up.sql` / `.down.sql` in `migrations/`; next number is **000968**. Seeds use `BEGIN; ... COMMIT;`.
- Go tests: `go test ./internal/wiki/...` — DB-backed tests use `opsTestStore(t)` and skip when `JC_DATABASE_URL` is unset; pure-function tests run unconditionally.
- Frontend tests: `cd apps/main && bun run test` (vitest). i18n keys go in `apps/main/src/lib/i18n/en.json` and `fr.json`.
- `course` wiki type stays offering-bound (`university_course_offerings`); new catalog type slug is `program`.
- Entity-bound wiki pages resolve at `/wiki/e/{type_slug}/{entity_slug}/{page_slug}`.

---

### Task 1: sqlc queries

**Files:**
- Modify: `internal/db/queries/wiki.sql` (extend 2 queries, add 3)
- Modify: `internal/db/queries/wiki_types.sql` (add 1 query)
- Generated: `internal/db/gen/wiki.sql.go`, `internal/db/gen/wiki_types.sql.go`

**Interfaces:**
- Produces: `gen.GetProgramForWikiRow{ID, Slug, Name}`, `gen.ListUniversitiesForCourseOfferingRow{ID, Slug, Name, GeoCode *string, OfferingSlug}`, `gen.ListUniversitiesForProgramRow{ID, Slug, Name, GeoCode *string, OfferingSlug}`, `gen.ListWikiEntityTypesForIDsRow{SpaceTypeID, TypeSlug, EntityKind *string, EntityDiscriminator []byte}`. `GetCourseOfferingForWikiRow`/`ListCourseOfferingsForWikiRow` gain `CourseID`/`ProgramSlug` fields — later tasks consume `ref`/`EntityChild` shapes, not these rows directly.

- [ ] **Step 1: Extend `GetCourseOfferingForWiki` + `ListCourseOfferingsForWiki` in `internal/db/queries/wiki.sql`**

Replace both queries (currently at ~lines 747–758) with:

```sql
-- name: GetCourseOfferingForWiki :one
SELECT o.id, o.slug, o.university_id, o.course_id,
       c.title AS name, c.slug AS program_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
WHERE o.slug = @slug AND o.deleted_at IS NULL AND o.is_published = true
LIMIT 1;

-- name: ListCourseOfferingsForWiki :many
SELECT o.id, o.slug, c.title AS name, c.slug AS program_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
WHERE o.university_id = @space_id AND o.deleted_at IS NULL AND o.is_published = true
ORDER BY c.title;
```

- [ ] **Step 2: Add the program + related-universities queries to `internal/db/queries/wiki.sql`** (immediately after `ListCourseOfferingsForWiki`)

```sql
-- name: GetProgramForWiki :one
-- Canonical catalog course (program wiki entity) by slug.
SELECT id, slug, title AS name
FROM university_course_catalog
WHERE slug = @slug AND deleted_at IS NULL AND is_published = true
LIMIT 1;

-- name: ListUniversitiesForCourseOffering :many
-- Sibling published offerings of the same catalog course → their host
-- universities ("also offered at" grid on an offering wiki page).
-- offering_slug is the per-uni course wiki page link.
SELECT s.id, s.slug, s.name, s.geo_code, MIN(o2.slug) AS offering_slug
FROM university_course_offerings o
JOIN university_course_offerings o2
  ON o2.course_id = o.course_id AND o2.id <> o.id
 AND o2.deleted_at IS NULL AND o2.is_published = true
JOIN spaces s ON s.id = o2.university_id
 AND s.deleted_at IS NULL AND s.status = 'active'
WHERE o.slug = @slug AND o.deleted_at IS NULL AND o.is_published = true
GROUP BY s.id, s.slug, s.name, s.geo_code
ORDER BY s.name
LIMIT 200;

-- name: ListUniversitiesForProgram :many
-- Every published offering of a catalog course → host universities
-- ("universities offering this program" grid on a program wiki page).
SELECT s.id, s.slug, s.name, s.geo_code, MIN(o.slug) AS offering_slug
FROM university_course_catalog c
JOIN university_course_offerings o
  ON o.course_id = c.id AND o.deleted_at IS NULL AND o.is_published = true
JOIN spaces s ON s.id = o.university_id
 AND s.deleted_at IS NULL AND s.status = 'active'
WHERE c.slug = @slug AND c.deleted_at IS NULL AND c.is_published = true
GROUP BY s.id, s.slug, s.name, s.geo_code
ORDER BY s.name
LIMIT 200;
```

- [ ] **Step 3: Add `ListWikiEntityTypesForIDs` to `internal/db/queries/wiki_types.sql`** (after `ListEntityWikiTypesForParent`, ~line 543)

```sql
-- name: ListWikiEntityTypesForIDs :many
-- Entity-bound wiki types among a resolved descendant-type id set —
-- computeDirectory and the directory `empty` check split space vs entity
-- children by entity_kind.
SELECT wtc.space_type_id, st.slug AS type_slug,
       wtc.entity_kind, wtc.entity_discriminator
FROM wiki_type_configs wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE wtc.space_type_id = ANY(@type_ids::uuid[])
  AND wtc.enabled = true AND wtc.entity_kind IS NOT NULL;
```

- [ ] **Step 4: Regenerate + build**

Run: `make sqlc-gen && go build ./internal/...`
Expected: clean build; `gen.GetProgramForWikiRow`, `gen.ListUniversitiesForCourseOfferingRow`, `gen.ListUniversitiesForProgramRow`, `gen.ListWikiEntityTypesForIDsRow` exist; `GetCourseOfferingForWikiRow` has `CourseID`/`ProgramSlug` fields.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/wiki.sql internal/db/queries/wiki_types.sql internal/db/gen/
git commit -m "feat(wiki): program + related-university queries for course cross-links"
```

---

### Task 2: Migration 000968 — `program` wiki type + category seeds

**Files:**
- Create: `migrations/000968_wiki_program_type_related_mode.up.sql`
- Create: `migrations/000968_wiki_program_type_related_mode.down.sql`

**Interfaces:**
- Produces: `wiki_entity_kinds` row `program`; `space_types` row slug `program` (id `00000000-0000-0000-0000-00000000007e`, `wiki_only=true`); `wiki_type_configs` entity_kind `program`; `program` categories `overview`/`universities`/`career-outcomes`; `course` category `universities` (position 5); university `programs` category rollup config `{"enabled":true,"mode":"directory","levels":["course"]}`.

- [ ] **Step 1: Write the up migration**

Follows the 000901 seed pattern (temp table → groups → categories → class assignments). `wiki_type_categories.rollup` is JSONB — pass-through.

```sql
-- 000968_wiki_program_type_related_mode.up.sql
-- University ↔ course/program cross-links (spec
-- 2026-10-30-wiki-university-course-program-links-design.md):
--   * 'program' entity kind + wiki_only type over
--     university_course_catalog (canonical "BSc Computer Engineering";
--     'course' stays offering-bound).
--   * program categories incl. 'universities' (rollup mode:"related").
--   * 'universities' category on the course (offering) type.
--   * university 'programs' category → directory-mode rollup over the
--     entity-bound 'course' child type (grid of offerings).
BEGIN;

INSERT INTO wiki_entity_kinds (entity_kind, source_table, id_column, slug_column, label) VALUES
    ('program', 'university_course_catalog', 'id', 'slug', 'Course Program')
ON CONFLICT (entity_kind) DO NOTHING;

INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-00000000007e', 'Program', 'program',
     'Canonical course/program (entity-bound wiki type)', 'book-open',
     NULL, true, NULL, true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator) VALUES
    ('00000000-0000-0000-0000-00000000007e', true, NULL, NULL, false, 18, 'program', NULL)
ON CONFLICT (space_type_id) DO NOTHING;
-- program gets NO wiki_type_parents edge: no single host space, and the
-- course→university edge already covers the child-listing axis.

CREATE TEMP TABLE _xlink_cat_seed (
    type_slug  text,
    slug       text,
    category   text,
    label      text,
    position   int,
    rollup     jsonb
) ON COMMIT DROP;

INSERT INTO _xlink_cat_seed (type_slug, slug, category, label, position, rollup) VALUES
    -- program (canonical catalog type)
    ('program', 'overview',        'overview',        'Overview',        1, NULL),
    ('program', 'universities',    'universities',    'Universities',    2, '{"enabled":true,"mode":"related"}'::jsonb),
    ('program', 'career-outcomes', 'career_outcomes', 'Career Outcomes', 3, NULL),
    -- course (offering type — positions continue past career-outcomes 4)
    ('course',  'universities',    'universities',    'Universities',    5, '{"enabled":true,"mode":"related"}'::jsonb);

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || c.type_slug || ')',
       'wiki_' || c.type_slug || '_' || c.category,
       true
FROM _xlink_cat_seed c
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup)
SELECT gen_random_uuid(), st.id, c.slug, c.category, c.label, g.id, c.position, true, c.rollup
FROM _xlink_cat_seed c
JOIN space_types st ON st.slug = c.type_slug
JOIN custom_field_groups g ON g.slug = 'wiki_' || c.type_slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Entity-class assignments so cfv writes scope to the entity rows.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, cfg.entity_kind, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN space_types st ON st.id = wtc.space_type_id
JOIN wiki_type_configs cfg ON cfg.space_type_id = wtc.space_type_id
JOIN _xlink_cat_seed s ON s.type_slug = st.slug AND s.slug = wtc.slug
WHERE cfg.entity_kind IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = wtc.field_group_id AND a.entity_class = cfg.entity_kind AND a.entity_id IS NULL
  );

-- University 'programs' category: directory-mode rollup listing the
-- entity-bound 'course' children (grid of this uni's offerings).
UPDATE wiki_type_categories c
SET rollup = '{"enabled":true,"mode":"directory","levels":["course"]}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug = 'university' AND c.slug = 'programs';

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000968_wiki_program_type_related_mode.down.sql
BEGIN;

UPDATE wiki_type_categories c
SET rollup = NULL
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug = 'university' AND c.slug = 'programs';

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN ('wiki_program_overview','wiki_program_universities',
                 'wiki_program_career_outcomes','wiki_course_universities');

DELETE FROM wiki_type_categories c
USING space_types st
WHERE c.space_type_id = st.id
  AND ((st.slug = 'program') OR (st.slug = 'course' AND c.slug = 'universities'));

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_program_overview','wiki_program_universities',
               'wiki_program_career_outcomes','wiki_course_universities');

DELETE FROM wiki_type_configs WHERE space_type_id = '00000000-0000-0000-0000-00000000007e';
DELETE FROM space_types WHERE id = '00000000-0000-0000-0000-00000000007e';
DELETE FROM wiki_entity_kinds WHERE entity_kind = 'program';

COMMIT;
```

- [ ] **Step 3: Apply + verify**

Run: `go run ./cmd/migrate/` (requires the dev DB up — `make docker-up` if needed).
Verify: `psql` — `SELECT slug FROM space_types WHERE slug='program';` returns 1 row; `SELECT slug, rollup FROM wiki_type_categories c JOIN space_types s ON s.id=c.space_type_id WHERE s.slug='program';` lists 3 categories.
If no DB is available, note it in the commit and let CI/integration run it.

- [ ] **Step 4: Commit**

```bash
git add migrations/000968_wiki_program_type_related_mode.up.sql migrations/000968_wiki_program_type_related_mode.down.sql
git commit -m "feat(wiki): seed program wiki type + universities/programs link categories"
```

---

### Task 3: `program` entity kind resolution

**Files:**
- Modify: `internal/wiki/entity.go` (`ResolveEntityRef`, ~line 55; `EntityRef` unchanged)
- Test: `internal/wiki/entity_test.go` (new file, or append to `service_hierarchy_test.go`)

**Interfaces:**
- Consumes: `gen.GetProgramForWikiRow` (Task 1).
- Produces: `Store.ResolveEntityRef(ctx, "program", slug)` → `EntityRef{Kind:"program", ID, Slug, Name, ParentSpaceID: invalid}`. Task 5 relies on `ref.ID`/`ref.Slug` for the program branch.

- [ ] **Step 1: Write the failing test (DB-backed, skips without `JC_DATABASE_URL`)**

```go
// TestResolveEntityRef_Program — the catalog entity kind resolves a
// published catalog row by slug, with no host space.
func TestResolveEntityRef_Program(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	mustIDF(t, ctx, database,
		`INSERT INTO university_course_catalog (id,title,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`,
		"Wop Program "+uniq, "wop-program-"+uniq)
	t.Cleanup(func() {
		database.Writer.Exec(t.Context(),
			`DELETE FROM university_course_catalog WHERE slug = $1`, "wop-program-"+uniq)
	})

	ref, err := store.ResolveEntityRef(ctx, "program", "wop-program-"+uniq)
	if err != nil {
		t.Fatalf("ResolveEntityRef: %v", err)
	}
	if ref.Name != "Wop Program "+uniq || ref.ParentSpaceID.Valid {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}
```

Run: `go test ./internal/wiki/ -run TestResolveEntityRef_Program -v` → FAIL (`unknown entity kind "program"`). If `JC_DATABASE_URL` is unset the test skips — also run `go build ./internal/wiki/` to see the missing case.

- [ ] **Step 2: Add the `program` branch in `ResolveEntityRef`** (`internal/wiki/entity.go`, in the kind switch after `case "course"`)

```go
	case "program":
		var r gen.GetProgramForWikiRow
		r, err = s.read.GetProgramForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.Slug = r.ID, r.Name, r.Slug
		}
```

Note: `ParentSpaceID` stays invalid — programs have no host space, so `attachEntityContext` yields empty ancestors (correct).

- [ ] **Step 3: Run test — PASS**

Run: `JC_DATABASE_URL=... go test ./internal/wiki/ -run TestResolveEntityRef_Program -v` and `go test ./internal/wiki/` (non-DB tests must stay green).

- [ ] **Step 4: Commit**

```bash
git add internal/wiki/entity.go internal/wiki/entity_test.go
git commit -m "feat(wiki): resolve program entity kind against course catalog"
```

---

### Task 4: Directory mode resolves entity-bound children (university → programs grid)

**Files:**
- Modify: `internal/wiki/entity.go` (`EntityChild` gains `ProgramSlug`; `ListEntityChildren` course branch maps it; extract `discriminatorValue` helper used by `entityChildren` + Task-4 service code)
- Modify: `internal/wiki/service_hierarchy.go` (`WikiDirectoryDTO` gains `EntityChildren`; `computeDirectory` splits space vs entity types)
- Modify: `internal/wiki/service_types.go` (`ListCategoriesForSpace` empty-check gains entity branch)
- Modify: `internal/wiki/store.go` (`ListWikiEntityTypesForIDs` store wrapper)
- Test: `internal/wiki/service_hierarchy_test.go`

**Interfaces:**
- Consumes: `gen.ListWikiEntityTypesForIDs`, `gen.ListCourseOfferingsForWikiRow.ProgramSlug` (Task 1).
- Produces:
  - `EntityChild{ID, Slug, Name, Discriminator, ProgramSlug string}` — `program_slug` in JSON as `program_slug,omitempty`.
  - `WikiDirectoryDTO.EntityChildren map[string][]EntityChild` — JSON `entity_children`, keyed by child type slug (e.g. `"course"`).
  - `Store.ListWikiEntityTypesForIDs(ctx, []pgtype.UUID) ([]gen.ListWikiEntityTypesForIDsRow, error)`.
  - `discriminatorValue(raw []byte) string` — first value of the `{col: val}` JSONB map; `""` on empty/nil.

- [ ] **Step 1: Write the failing test**

In `service_hierarchy_test.go` — seeds a university-type space, an entity-bound child type (`entity_kind='course'`), a parent edge, and an offering row; asserts `computeDirectory` returns it under `EntityChildren["course"]`.

```go
// TestComputeDirectory_EntityChildren — a directory-mode category whose
// levels include an entity-bound type lists that kind's entity rows.
func TestComputeDirectory_EntityChildren(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	// Host space + entity-bound child type declared as its descendant.
	spaceID := mustIDF(t, ctx, database,
		`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status)
		 VALUES (gen_random_uuid(),$1,$2,$2,'custom','public','active') RETURNING id`,
		hf.typeID, "wop-dir-"+uniq)
	childType := mustIDF(t, ctx, database,
		`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-ent-"+uniq)
	mustIDF(t, ctx, database,
		`INSERT INTO wiki_type_configs (space_type_id,enabled,entity_kind) VALUES ($1,true,'course') RETURNING space_type_id`, childType)
	mustIDF(t, ctx, database,
		`INSERT INTO wiki_type_parents (space_type_id,parent_space_type_id) VALUES ($1,$2) RETURNING space_type_id`, childType, hf.typeID)

	// catalog + offering hosted by the space.
	courseID := mustIDF(t, ctx, database,
		`INSERT INTO university_course_catalog (id,title,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`,
		"Wop Program "+uniq, "wop-program-"+uniq)
	mustIDF(t, ctx, database,
		`INSERT INTO university_course_offerings (id,course_id,university_id,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,$3,true) RETURNING id`,
		courseID, spaceID, "wop-offering-"+uniq)

	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM university_course_offerings WHERE slug = $1`, "wop-offering-"+uniq)
		database.Writer.Exec(cctx, `DELETE FROM university_course_catalog WHERE slug = $1`, "wop-program-"+uniq)
		database.Writer.Exec(cctx, `DELETE FROM spaces WHERE id = $1`, spaceID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_parents WHERE space_type_id = $1`, childType)
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childType)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, childType)
	})

	svc := NewService(store, nil)
	d := svc.computeDirectory(ctx, spaceID, hf.typeID,
		&WikiRollupConfig{Enabled: true, Mode: rollupModeDirectory, Levels: []string{"wop-ent-" + uniq}})
	if d == nil {
		t.Fatal("directory nil for entity-child config")
	}
	kids := d.EntityChildren["wop-ent-"+uniq]
	if len(kids) != 1 || kids[0].Slug != "wop-offering-"+uniq {
		t.Fatalf("entity children: %+v", kids)
	}
	if kids[0].ProgramSlug != "wop-program-"+uniq {
		t.Fatalf("program slug missing: %+v", kids[0])
	}
}
```

- [ ] **Step 2: `EntityChild` + `ListEntityChildren` changes** (`internal/wiki/entity.go`)

```go
// EntityChild is a destination-tree row: one entity child of a host space.
type EntityChild struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Discriminator string `json:"discriminator,omitempty"`
	// ProgramSlug links a course offering to its canonical catalog wiki
	// page (/wiki/e/program/{slug}) — empty for other kinds.
	ProgramSlug   string `json:"program_slug,omitempty"`
}
```

Course branch of `ListEntityChildren` becomes:

```go
		out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name, ProgramSlug: r.ProgramSlug})
```

Extract the discriminator decode used in `entityChildren` (lines ~440–449) into a package helper and reuse it there:

```go
// discriminatorValue returns the first value of an entity_discriminator
// {column: value} map; "" when empty or malformed.
func discriminatorValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, v := range m {
		return v
	}
	return ""
}
```

- [ ] **Step 3: Store wrapper** (`internal/wiki/store.go`, near `ListDirectoryChildren`)

```go
// ListWikiEntityTypesForIDs returns the entity-bound wiki types among a
// resolved descendant-type set (slug + entity_kind + discriminator) —
// computeDirectory splits space vs entity children by entity_kind.
func (s *Store) ListWikiEntityTypesForIDs(ctx context.Context, typeIDs []pgtype.UUID) ([]gen.ListWikiEntityTypesForIDsRow, error) {
	return s.read.ListWikiEntityTypesForIDs(ctx, typeIDs)
}
```

- [ ] **Step 4: `WikiDirectoryDTO` + `computeDirectory` split** (`internal/wiki/service_hierarchy.go`)

```go
type WikiDirectoryDTO struct {
	Levels   []string                `json:"levels"`
	Children []WikiDirectoryChildDTO `json:"children"`
	// EntityChildren lists entity-bound descendants (course offerings on
	// a university) keyed by child type slug — resolved via
	// ListEntityChildren, not the space hierarchy.
	EntityChildren map[string][]EntityChild `json:"entity_children,omitempty"`
	Total    int                     `json:"total"`
}
```

In `computeDirectory`, after `typeIDs` resolves (line ~223), split and fetch entity children:

```go
	// Entity-bound descendant types resolve via entity tables — the
	// space-hierarchy query can't see them.
	entTypes, terr := s.store.ListWikiEntityTypesForIDs(ctx, typeIDs)
	if terr != nil {
		slog.Warn("wiki: directory: entity types failed", "error", terr)
	}
	entIDs := make(map[pgtype.UUID]bool, len(entTypes))
	entityChildren := map[string][]EntityChild{}
	for _, et := range entTypes {
		entIDs[et.SpaceTypeID] = true
		if et.EntityKind == nil {
			continue
		}
		kids, kerr := s.store.ListEntityChildren(ctx, spaceID, *et.EntityKind, discriminatorValue(et.EntityDiscriminator))
		if kerr != nil {
			slog.Warn("wiki: directory: entity children failed", "type", et.TypeSlug, "error", kerr)
			continue
		}
		if len(kids) > 0 {
			entityChildren[et.TypeSlug] = kids
		}
	}
	spaceIDs := make([]pgtype.UUID, 0, len(typeIDs))
	for _, id := range typeIDs {
		if !entIDs[id] {
			spaceIDs = append(spaceIDs, id)
		}
	}

	// Guard: ListWikiDirectoryChildren treats cardinality(@type_ids)=0 as
	// "no type filter" — an all-entity type set would list unrelated
	// descendants, not none.
	var children []WikiDirectoryChild
	if len(spaceIDs) > 0 {
		children, err = s.store.ListDirectoryChildren(ctx, spaceID, spaceIDs)
		if err != nil {
			slog.Warn("wiki: directory: children failed",
				"space_id", spaceID.String(), "error", err)
			return nil
		}
	}
	if len(children) == 0 && len(entityChildren) == 0 {
		return nil
	}
```

Then in the `seen`/levels loop, also append each `et.TypeSlug` from `entTypes` (entity groups get a filter chip too), include `len`-sum in `Total`, and set `EntityChildren: entityChildren` (or nil when empty) on the returned DTO.

- [ ] **Step 5: `ListCategoriesForSpace` empty-check** (`internal/wiki/service_types.go`, ~line 404)

After `typeIDs, terr := s.rollupTypeIDs(...)`, replace the `HasDirectoryChildren` call with:

```go
		spaceIDs := typeIDs
		has := false
		if entTypes, terr := s.store.ListWikiEntityTypesForIDs(ctx, typeIDs); terr == nil {
			entIDs := make(map[pgtype.UUID]bool, len(entTypes))
			for _, et := range entTypes {
				entIDs[et.SpaceTypeID] = true
				if et.EntityKind == nil {
					continue
				}
				kids, kerr := s.store.ListEntityChildren(ctx, spaceID, *et.EntityKind, discriminatorValue(et.EntityDiscriminator))
				if kerr == nil && len(kids) > 0 {
					has = true
				}
			}
			spaceIDs = make([]pgtype.UUID, 0, len(typeIDs))
			for _, id := range typeIDs {
				if !entIDs[id] {
					spaceIDs = append(spaceIDs, id)
				}
			}
		}
		// Same empty-array guard as computeDirectory: cardinality 0 means
		// unfiltered, not "none" — skip the space query when every
		// resolved type is entity-bound.
		if !has && len(spaceIDs) > 0 {
			h, herr := s.store.HasDirectoryChildren(ctx, spaceID, spaceIDs)
			has = herr == nil && h
		}
		cats[i].Empty = !has
```

- [ ] **Step 6: Run tests + build**

Run: `go build ./internal/... && go test ./internal/wiki/` and with DB: `JC_DATABASE_URL=... go test ./internal/wiki/ -run TestComputeDirectory_EntityChildren -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/wiki/
git commit -m "feat(wiki): directory mode lists entity-bound children"
```

---

### Task 5: `mode:"related"` — peer universities on entity pages

**Files:**
- Modify: `internal/wiki/service_hierarchy.go` (`rollupModeRelated` const)
- Modify: `internal/wiki/service_types.go` (`validateRollupInput` mode check)
- Modify: `internal/wiki/entity.go` (`WikiRelatedSpaceDTO`, `EntityRelatedDTO`, `EntityPageDTO.Related`, `computeEntityRelated`, attach in `attachEntityContext` + `entityLazyPage`)
- Test: `internal/wiki/service_types_test.go` (validation unit test), `internal/wiki/entity_test.go` (resolver DB test)

**Interfaces:**
- Consumes: `gen.ListUniversitiesForCourseOffering`, `gen.ListUniversitiesForProgram` (Task 1); `EntityRef` (Task 3).
- Produces:
  - `const rollupModeRelated = "related"` (in `service_hierarchy.go` beside the other mode consts).
  - `WikiRelatedSpaceDTO{Slug, Name string, GeoCode *string, OfferingSlug string}` — JSON `{slug, name, geo_code?, offering_slug?}`.
  - `EntityRelatedDTO{Kind string, Items []WikiRelatedSpaceDTO, Total int}` — JSON `{kind, items, total}`.
  - `EntityPageDTO.Related *EntityRelatedDTO` — JSON `related,omitempty`.
  - `Service.computeEntityRelated(ctx, kind string, ref EntityRef) *EntityRelatedDTO` — `course`→sibling unis, `program`→all offering unis, `Kind:"universities"`; nil otherwise/on error/empty.

- [ ] **Step 1: Write the failing tests**

Unit (`service_types_test.go`, beside `TestValidateRollupInput`):

```go
func TestValidateRollupInput_RelatedMode(t *testing.T) {
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"related"}`)); err != nil {
		t.Fatalf("related mode rejected: %v", err)
	}
}
```

DB (`entity_test.go`):

```go
// TestComputeEntityRelated_CourseSiblings — an offering's related set is
// the OTHER universities offering the same catalog course.
func TestComputeEntityRelated_CourseSiblings(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	mkSpace := func(n string) pgtype.UUID {
		return mustIDF(t, ctx, database,
			`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status)
			 VALUES (gen_random_uuid(),$1,$2,$3,'custom','public','active') RETURNING id`,
			hf.typeID, n, n)
	}
	u1, u2 := mkSpace("wop-uni-a-"+uniq), mkSpace("wop-uni-b-"+uniq)
	courseID := mustIDF(t, ctx, database,
		`INSERT INTO university_course_catalog (id,title,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`, "Wop Program "+uniq, "wop-prog-"+uniq)
	for _, tc := range []struct{ uni pgtype.UUID; slug string }{
		{u1, "wop-off-a-" + uniq}, {u2, "wop-off-b-" + uniq},
	} {
		mustIDF(t, ctx, database,
			`INSERT INTO university_course_offerings (id,course_id,university_id,slug,is_published)
			 VALUES (gen_random_uuid(),$1,$2,$3,true) RETURNING id`, courseID, tc.uni, tc.slug)
	}
	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM university_course_offerings WHERE course_id = $1`, courseID)
		database.Writer.Exec(cctx, `DELETE FROM university_course_catalog WHERE id = $1`, courseID)
		database.Writer.Exec(cctx, `DELETE FROM spaces WHERE id IN ($1, $2)`, u1, u2)
	})

	svc := NewService(store, nil)

	rel := svc.computeEntityRelated(ctx, "course", EntityRef{Kind: "course", Slug: "wop-off-a-" + uniq})
	if rel == nil || len(rel.Items) != 1 || rel.Items[0].Slug != "wop-uni-b-"+uniq {
		t.Fatalf("course related: %+v", rel)
	}
	if rel.Items[0].OfferingSlug != "wop-off-b-"+uniq {
		t.Fatalf("offering slug missing: %+v", rel.Items[0])
	}

	rel = svc.computeEntityRelated(ctx, "program", EntityRef{Kind: "program", Slug: "wop-prog-" + uniq})
	if rel == nil || rel.Total != 2 {
		t.Fatalf("program related: %+v", rel)
	}
}
```

Run: `go test ./internal/wiki/` → both FAIL.

- [ ] **Step 2: Mode const + validation**

In `service_hierarchy.go` beside `rollupModeDirectory`/`rollupModePages`:

```go
// rollupModeRelated switches an entity-bound category rollup to a peer
// listing resolved through the entity kind — course → sibling-offering
// universities, program → all offering universities. Meaningless on
// space-bound categories (compute ignores it there).
const rollupModeRelated = "related"
```

In `validateRollupInput` (`service_types.go` ~line 854), extend the mode check:

```go
	if cfg.Mode != "" && cfg.Mode != rollupModeDirectory && cfg.Mode != rollupModePages && cfg.Mode != rollupModeRelated {
```

- [ ] **Step 3: DTOs + resolver** (`internal/wiki/entity.go`, after `EntityPageDTO`)

```go
// WikiRelatedSpaceDTO is one card in an entity's related-universities
// grid — the space identity plus the linking offering's wiki slug.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRelatedSpaceDTO struct {
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	GeoCode      *string `json:"geo_code,omitempty"`
	OfferingSlug string  `json:"offering_slug,omitempty"`
}

// EntityRelatedDTO is the peer listing attached to a mode:"related"
// entity category page. Kind names the relation ("universities").
// //nolint:revive // exported: DTO naming follows package convention.
type EntityRelatedDTO struct {
	Kind  string                `json:"kind"`
	Items []WikiRelatedSpaceDTO `json:"items"`
	Total int                   `json:"total"`
}
```

`EntityPageDTO` gains:

```go
	// Related is the mode:"related" peer listing (course → sibling-offering
	// universities, program → all offering universities); nil when the
	// category has no related config or the listing is empty.
	Related         *EntityRelatedDTO    `json:"related,omitempty"`
```

Resolver (same file):

```go
// computeEntityRelated resolves an entity-bound type's peer listing:
// course → sibling published offerings' universities (self excluded),
// program → every published offering's universities. Fails open — nil
// on lookup error or empty so the page renders without the grid.
func (s *Service) computeEntityRelated(ctx context.Context, kind string, ref EntityRef) *EntityRelatedDTO {
	dto := &EntityRelatedDTO{Kind: "universities"}
	switch kind {
	case "course":
		rows, err := s.store.read.ListUniversitiesForCourseOffering(ctx, ref.Slug)
		if err != nil {
			slog.Warn("wiki: related: course siblings failed", "error", err)
			return nil
		}
		for _, r := range rows {
			dto.Items = append(dto.Items, WikiRelatedSpaceDTO{
				Slug: r.Slug, Name: r.Name, GeoCode: r.GeoCode, OfferingSlug: r.OfferingSlug,
			})
		}
	case "program":
		rows, err := s.store.read.ListUniversitiesForProgram(ctx, ref.Slug)
		if err != nil {
			slog.Warn("wiki: related: program universities failed", "error", err)
			return nil
		}
		for _, r := range rows {
			dto.Items = append(dto.Items, WikiRelatedSpaceDTO{
				Slug: r.Slug, Name: r.Name, GeoCode: r.GeoCode, OfferingSlug: r.OfferingSlug,
			})
		}
	default:
		return nil
	}
	if len(dto.Items) == 0 {
		return nil
	}
	dto.Total = len(dto.Items)
	return dto
}
```

- [ ] **Step 4: Attach — materialized path** (`attachEntityContext` in `entity.go`, after the ancestors block ~line 307)

```go
	// mode:"related" categories attach the entity's peer listing — the
	// category row's rollup decides, so the grid only lives on pages
	// whose category declares it (e.g. 'universities').
	if cat, err := s.store.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
		SpaceTypeID: typeID, Token: dto.Category,
	}); err == nil {
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && rcfg.Mode == rollupModeRelated {
			dto.Related = s.computeEntityRelated(ctx, kind, ref)
		}
	}
```

- [ ] **Step 5: Attach — lazy path** (`entityLazyPage`, in the category-match branch ~line 373, after `base.Category = cat.Category`)

```go
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && rcfg.Mode == rollupModeRelated {
			base.Related = s.computeEntityRelated(ctx, kind, ref)
		}
```

- [ ] **Step 6: Run tests + build**

Run: `go build ./internal/... && go test ./internal/wiki/` and `JC_DATABASE_URL=... go test ./internal/wiki/ -run 'TestComputeEntityRelated|TestResolveEntityRef_Program|TestComputeDirectory' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/wiki/
git commit -m "feat(wiki): mode:related peer universities on entity pages"
```

---

### Task 6: Frontend — directory panel renders entity children

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (`WikiDirectory` gains `entity_children`; new `WikiEntityChild`)
- Create: `apps/main/src/lib/utils/wikiEntityLinks.ts` + `apps/main/src/lib/utils/wikiEntityLinks.test.ts`
- Modify: `apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json`

**Interfaces:**
- Consumes: `WikiDirectoryDTO.entity_children` (Task 4) — `{type_slug → [{id, slug, name, program_slug?}]}`.
- Produces:
  - `export interface WikiEntityChild { id: string; slug: string; name: string; discriminator?: string; program_slug?: string }`
  - `WikiDirectory.entity_children?: Record<string, WikiEntityChild[]>`
  - `entityPageHref(typeSlug: string, slug: string): string` → `/wiki/e/{type}/{slug}/overview` (encodeURIComponent each segment).

- [ ] **Step 1: Write the failing test** (`apps/main/src/lib/utils/wikiEntityLinks.test.ts`)

```ts
import { describe, expect, it } from 'vitest';
import { entityPageHref } from './wikiEntityLinks';

describe('entityPageHref', () => {
	it('builds the entity overview path', () => {
		expect(entityPageHref('course', 'msc-cs-unilag')).toBe('/wiki/e/course/msc-cs-unilag/overview');
	});
	it('encodes segments', () => {
		expect(entityPageHref('student_visa', 'a b')).toBe('/wiki/e/student_visa/a%20b/overview');
	});
});
```

Create `apps/main/src/lib/utils/wikiEntityLinks.ts`:

```ts
// Entity-bound wiki page links (JCE ↔ wiki mapping): /wiki/e/{type}/{entity}/{page}.
export function entityPageHref(typeSlug: string, entitySlug: string, pageSlug = 'overview'): string {
	return `/wiki/e/${encodeURIComponent(typeSlug)}/${encodeURIComponent(entitySlug)}/${encodeURIComponent(pageSlug)}`;
}
```

Run: `cd apps/main && bun run test` → PASS.

- [ ] **Step 2: Types** (`apps/main/src/lib/types/index.ts`, beside `WikiDirectoryChild`)

```ts
/** §wiki-links — one entity-bound directory child (a university's course
 *  offering); program_slug links the canonical program wiki when set. */
export interface WikiEntityChild {
	id: string;
	slug: string;
	name: string;
	discriminator?: string;
	program_slug?: string;
}
```

`WikiDirectory` gains:

```ts
	/** §wiki-links — entity-bound descendants keyed by child type slug
	 *  (e.g. {course: [...]}) — resolved outside the space hierarchy. */
	entity_children?: Record<string, WikiEntityChild[]>;
```

- [ ] **Step 3: Panel rendering** (`WikiDirectoryPanel.svelte`)

Script additions:

```ts
	import { entityPageHref } from '$lib/utils/wikiEntityLinks';

	const entityGroups = $derived(
		Object.entries(directory.entity_children ?? {}).filter(([, items]) => items.length > 0)
	);
```

Markup after the `{#each [...groups]}` block (before the truncated note) — entity cards can't nest a second `<a>` inside the card link, so the card is a `div` with two links:

```svelte
	{#each entityGroups as [typeSlug, items] (typeSlug)}
		<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			{typeLabel(typeSlug)}
		</h2>
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each items as child (child.id)}
				<div class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]">
					<a
						href={entityPageHref(typeSlug, child.slug)}
						class="text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
						data-sveltekit-preload-data="hover"
					>
						{child.name}
					</a>
					{#if child.program_slug}
						<a
							href={entityPageHref('program', child.program_slug)}
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
						>
							{t('wiki.view_canonical_program', 'Canonical program')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	{/each}
```

- [ ] **Step 4: i18n keys**

`en.json` (alphabetical, near `wiki.directory_*`):

```json
	"wiki.also_offered_at": "Also offered at",
	"wiki.universities_offering": "Universities offering this program",
	"wiki.view_at_university": "View at {name}",
	"wiki.view_canonical_program": "Canonical program",
	"wiki.type_program": "Programs",
```

`fr.json`:

```json
	"wiki.also_offered_at": "Également proposé à",
	"wiki.universities_offering": "Universités proposant ce programme",
	"wiki.view_at_university": "Voir à {name}",
	"wiki.view_canonical_program": "Programme canonique",
	"wiki.type_program": "Programmes",
```

- [ ] **Step 5: Verify + commit**

Run: `cd apps/main && bun run test && bun run check` (svelte-check if configured as `check`; else `bun run build` typecheck).
Expected: clean.

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/lib/utils/wikiEntityLinks.ts apps/main/src/lib/utils/wikiEntityLinks.test.ts apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(wiki): render entity children in directory panels"
```

---

### Task 7: Frontend — related-universities grid on entity pages

**Files:**
- Modify: `apps/main/src/routes/(app)/wiki/e/[type_slug]/[entity]/[page_slug]/+page.server.ts` (interface + pass-through)
- Modify: `apps/main/src/routes/(app)/wiki/e/[type_slug]/[entity]/[page_slug]/+page.svelte` (grid)

**Interfaces:**
- Consumes: `EntityPageDTO.related` (Task 5) — `{kind: "universities", items: [{slug, name, geo_code?, offering_slug?}], total}`; `entityPageHref` (Task 6); `page.kind` (`'course'`|`'program'`).

- [ ] **Step 1: Extend the route interface** (`+page.server.ts` `EntityWikiPage`)

```ts
	/** §wiki-links — mode:"related" peer listing (universities). */
	related?: {
		kind: string;
		items: { slug: string; name: string; geo_code?: string; offering_slug?: string }[];
		total: number;
	};
```

No load change — the API response already carries `related` through `apiGet`/`{data: page}`.

- [ ] **Step 2: Render the grid** (`+page.svelte`)

Script:

```ts
	import { entityPageHref } from '$lib/utils/wikiEntityLinks';

	const relatedHeading = $derived(
		page?.kind === 'course'
			? t('wiki.also_offered_at', 'Also offered at')
			: t('wiki.universities_offering', 'Universities offering this program')
	);
```

Markup after the fields `</section>` (before `</article>`):

```svelte
	{#if page.related && page.related.items.length > 0}
		<section class="space-y-3">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{relatedHeading}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each page.related.items as uni (uni.slug)}
					<div class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]">
						<a
							href="/wiki/{encodeURIComponent(uni.slug)}"
							class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
							data-sveltekit-preload-data="hover"
						>
							{uni.name}
							{#if uni.geo_code}
								<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{uni.geo_code}</span>
							{/if}
						</a>
						{#if uni.offering_slug}
							<a
								href={entityPageHref('course', uni.offering_slug)}
								class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
							>
								{t('wiki.view_at_university', 'View at {name}', { name: uni.name })}
							</a>
						{/if}
					</div>
				{/each}
			</div>
			{#if page.related.total > page.related.items.length}
				<p class="text-xs text-[var(--color-muted)]">
					{t('wiki.directory_truncated', 'Showing {shown} of {total} — search to find the rest.', {
						shown: page.related.items.length,
						total: page.related.total
					})}
				</p>
			{/if}
		</section>
	{/if}
```

- [ ] **Step 3: Verify + commit**

Run: `cd apps/main && bun run check` (or `bun run build`) — clean typecheck.

```bash
git add "apps/main/src/routes/(app)/wiki/e/[type_slug]/[entity]/[page_slug]/"
git commit -m "feat(wiki): related-universities grid on course/program pages"
```

---

### Task 8: End-to-end verification

**Files:** none (manual/verification task)

- [ ] **Step 1: Seed or pick fixtures**

With the dev DB + API running: create (or locate) one university space, one catalog course, and two published offerings at two universities.

- [ ] **Step 2: Verify the three surfaces**

- `GET /api/v1/wiki/categories?space={uni-slug}` → `programs` category not `empty`.
- `GET /api/v1/wiki/{uni-slug}/programs` → `data.directory.entity_children.course` lists the offerings with `program_slug`.
- `GET /api/v1/wiki/e/course/{offering-a}/universities` → `data.related.items` = the other university.
- `GET /api/v1/wiki/e/program/{catalog-slug}/universities` → `data.related.items` = both universities.
- Browser: `/wiki/{uni-slug}/programs` shows the card grid linking to offering pages + canonical program sub-link; `/wiki/e/program/{slug}/universities` shows uni cards with both link targets.

- [ ] **Step 3: Regression sweep**

- `go test ./internal/wiki/` green; `cd apps/main && bun run test` green.
- A directory category with only space levels (e.g. country→states) still renders identically — `entity_children` absent.
- A university with zero offerings hides the `programs` tab (`empty: true`).

- [ ] **Step 4: Commit** (if any fixes were needed)

```bash
git commit -m "fix(wiki): e2e corrections for university↔program links"
```
