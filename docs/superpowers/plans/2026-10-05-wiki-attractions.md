# Wiki Attractions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Attractions as a wiki-only space type — seeded type + categories, image-first directory cards on parent wikis, gallery field rendering, Wikipedia/Wikidata auto-enrichment, and a bulk seeder task + admin trigger.

**Architecture:** Reuses the §80 wiki stack end-to-end. `space_types.wiki_only` gates the community surface; `attraction` is a wiki type with `wiki_type_parents` → city/state/country; parents gain an `attractions` directory-mode category; enrichment extends `AutoPopulateSpace`; bulk seeding is a new `wiki:seed_attractions` Asynq task.

**Tech Stack:** Go (chi, pgx, sqlc, asynq), PostgreSQL, SvelteKit 2 + Svelte 5 runes (apps/main SSR, apps/admin SPA), Wikidata/Wikipedia APIs.

## Global Constraints

- **Never hand-edit `internal/db/gen/`** — edit `internal/db/queries/*.sql`, run `make sqlc-gen` (Rule 2).
- **Commits to `main` branch only.**
- **No AI attribution anywhere** — commit messages and comments describe what/why, never authorship (Rule 17).
- **`bun` only** for frontend tooling — `bunx svelte-check`, `bun run check` (Rule 18).
- **Migrations are paired up/down**, `golang-migrate` numbering; next free: **000795** (highest present: 000794).
- **No hardcoded strings that belong in config** (Rule 3); Valkey keys use `jc:` prefix from the catalog (Rule 4).
- **API errors** use `errpkg.WriteError(w, status, "snake_case_code", "...")` (Rule 5).
- Spec: `docs/superpowers/specs/2026-10-05-wiki-attractions-design.md`; JEOS: `.jeos/sections/80-destination-wiki.md` (Wiki-Only Space Types & Attractions amendment).
- Space type IDs (fixed, from baseline seed): country `…060`, state `…061`, city `…062`, university `…064`. **New:** attraction `00000000-0000-0000-0000-000000000071`.
- `wiki_type_categories.field_group_id` is **NOT NULL** — every category needs a field group, even an empty one (directory/prose-only categories get `wiki_{type}_{category}` groups with no fields).
- Every category's group must get a `custom_field_group_assignments` row scoped to `entity_class = 'space_type:{slug}'`, `entity_id NULL` (pattern in `migrations/000739`, tail block).
- **Attraction overview fields get deterministic UUIDs** (`00000000-0000-0000-0000-000000000111`–`…000121`; highest baseline `…000110`) so enrichment + seed-dedup address them without lookups — the `universityFieldUUIDs` convention (processoradapter.go:553).
- `select` values store `{"value": "museum"}` in `value_json` (NOT `value_text`); `image_multi`/`media` store `{"media_ids": ["<uuid>", …]}` — a new convention this feature defines; read side renders `{"items":[{"id","url","alt","full_url"}]}`.
- `wiki_content` carries `{"wikidata_id": "Q…"}` for seeded attractions; `populateAttractionSpace` must preserve that key when overwriting with the summary JSON (merge, don't replace).

---

### Task 1: Migration + sqlc regen (wiki_only column, attraction seeds)

**Files:**
- Create: `migrations/000795_space_types_wiki_only_attractions.up.sql`
- Create: `migrations/000795_space_types_wiki_only_attractions.down.sql`
- Modify: `internal/db/queries/spaces.sql:20-63` (add `wiki_only` to all space_types column lists + `UpdateSpaceType` SET)

**Interfaces:**
- Produces: `gen.SpaceType.WikiOnly bool`; `gen.UpdateSpaceTypeParams.WikiOnly pgtype.Bool`; `space_types.wiki_only`; seeded `attraction` type (`…071`), `wiki_type_configs` row, `wiki_type_parents` → 060/061/062, `overview` + `plan-your-visit` categories on attraction, `attractions` directory categories on 060/061/062.

- [ ] **Step 1: Write the up migration**

```sql
-- 000795_space_types_wiki_only_attractions.up.sql
-- §80 Wiki-Only Space Types & Attractions amendment: space_types.wiki_only
-- marks a type's spaces as wiki-surface-only (no feed/join/posting). Seeds
-- the 'attraction' wiki type (child of city/state/country) with an
-- overview + plan-your-visit category template, and adds an 'attractions'
-- directory-mode category to country/state/city.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      docs/superpowers/specs/2026-10-05-wiki-attractions-design.md

BEGIN;

ALTER TABLE space_types ADD COLUMN wiki_only boolean NOT NULL DEFAULT false;

INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-000000000071', 'Attraction', 'attraction',
     'Tourist attraction or natural wonder — wiki-only space', 'landmark',
     NULL, true, 'geo', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position) VALUES
    ('00000000-0000-0000-0000-000000000071', true, 'Attractions', 'landmark', true, 5)
ON CONFLICT (space_type_id) DO NOTHING;

INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id) VALUES
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000060'), -- attraction → country
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000061'), -- attraction → state
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000062')  -- attraction → city
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Category seeds: two sets.
--  a) attraction's own template: overview (profile + gallery) and
--     plan-your-visit (prose). Groups get real field defs.
--  b) 'attractions' directory-mode categories on country/state/city —
--     rollup.mode='directory', levels=['attraction']; empty groups.
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE _attr_cat_seed (
    type_id  uuid,
    slug     text,
    category text,
    label    text,
    position int,
    rollup   jsonb,
    icon     text
);

INSERT INTO _attr_cat_seed (type_id, slug, category, label, position, rollup, icon) VALUES
    -- attraction type template
    ('00000000-0000-0000-0000-000000000071', 'overview',        'overview',        'Overview',        1, NULL, 'info'),
    ('00000000-0000-0000-0000-000000000071', 'plan-your-visit', 'plan_your_visit', 'Plan Your Visit', 2, NULL, 'map'),
    -- directory categories on the geo chain (after existing positions)
    ('00000000-0000-0000-0000-000000000060', 'attractions', 'attractions', 'Attractions', 9,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark'),
    ('00000000-0000-0000-0000-000000000061', 'attractions', 'attractions', 'Attractions', 7,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark'),
    ('00000000-0000-0000-0000-000000000062', 'attractions', 'attractions', 'Attractions', 7,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark');

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || st.slug || ')',
       'wiki_' || st.slug || '_' || c.category,
       true
FROM _attr_cat_seed c
JOIN space_types st ON st.id = c.type_id
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
SELECT gen_random_uuid(), c.type_id, c.slug, c.category, c.label,
       g.id, c.position, true, c.rollup, c.icon
FROM _attr_cat_seed c
JOIN space_types st ON st.id = c.type_id
JOIN custom_field_groups g ON g.slug = 'wiki_' || st.slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, 'space_type:' || st.slug, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = wtc.field_group_id
      AND a.entity_class = 'space_type:' || st.slug
      AND a.entity_id IS NULL
);

-- Attraction overview field schema (wiki_attraction_overview group).
-- Deterministic UUIDs (…0111–…0121, next free after baseline …0110) so
-- enrichAttractionFields + the seed dedup upsert can address fields without
-- a lookup — mirrors universityFieldUUIDs (processoradapter.go).
INSERT INTO custom_fields (id, group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT v.id, g.id, v.label, v.key, v.field_type, v.options, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('00000000-0000-0000-0000-000000000111'::uuid, 'Kind',           'kind',          'select',      '[{"value":"museum","label":"Museum"},{"value":"landmark","label":"Landmark"},{"value":"monument","label":"Monument"},{"value":"historic-site","label":"Historic Site"},{"value":"religious-site","label":"Religious Site"},{"value":"market","label":"Market"},{"value":"waterfall","label":"Waterfall"},{"value":"mountain","label":"Mountain"},{"value":"rock-formation","label":"Rock Formation"},{"value":"lake","label":"Lake"},{"value":"cave","label":"Cave"},{"value":"beach","label":"Beach"},{"value":"forest-reserve","label":"Forest Reserve"},{"value":"canyon","label":"Canyon"},{"value":"hot-spring","label":"Hot Spring"},{"value":"national-park","label":"National Park"},{"value":"garden","label":"Garden"},{"value":"zoo","label":"Zoo"},{"value":"theme-park","label":"Theme Park"},{"value":"other","label":"Other"}]'::jsonb, true,  1),
    ('00000000-0000-0000-0000-000000000112'::uuid, 'Address',        'address',       'text',        NULL::jsonb, false, 2),
    ('00000000-0000-0000-0000-000000000113'::uuid, 'Coordinates',    'coordinates',   'text',        NULL::jsonb, false, 3),
    ('00000000-0000-0000-0000-000000000114'::uuid, 'Opening Hours',  'opening_hours', 'textarea',    NULL::jsonb, false, 4),
    ('00000000-0000-0000-0000-000000000115'::uuid, 'Entry Fee',      'entry_fee',     'price_range', NULL::jsonb, false, 5),
    ('00000000-0000-0000-0000-000000000116'::uuid, 'Website',        'website',       'url',         NULL::jsonb, false, 6),
    ('00000000-0000-0000-0000-000000000117'::uuid, 'Wikidata ID',    'wikidata_id',   'text',        NULL::jsonb, false, 7),
    ('00000000-0000-0000-0000-000000000118'::uuid, 'Gallery',        'gallery',       'image_multi', '{"max_files":12,"max_size_mb":8}'::jsonb, false, 8)
) AS v(id, label, key, field_type, options, filterable, position)
WHERE g.slug = 'wiki_attraction_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Plan Your Visit field schema (wiki_attraction_plan_your_visit group).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, 'textarea', NULL::jsonb, false, true, true, false, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Getting There', 'getting_there', 1),
    ('Best Time',     'best_time',     2),
    ('Tips',          'tips',          3)
) AS v(label, key, position)
WHERE g.slug = 'wiki_attraction_plan_your_visit'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

DROP TABLE _attr_cat_seed;

COMMIT;
```

> Note: the group slug for `plan-your-visit` is `wiki_attraction_plan_your_visit` because the migration derives it from `category` (underscored), not `slug`.

- [ ] **Step 2: Write the down migration**

```sql
-- 000795_space_types_wiki_only_attractions.down.sql

BEGIN;

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN (
      'wiki_attraction_overview', 'wiki_attraction_plan_your_visit',
      'wiki_country_attractions', 'wiki_state_attractions', 'wiki_city_attractions'
  );

DELETE FROM wiki_type_categories
WHERE (space_type_id = '00000000-0000-0000-0000-000000000071')
   OR (slug = 'attractions' AND space_type_id IN (
       '00000000-0000-0000-0000-000000000060',
       '00000000-0000-0000-0000-000000000061',
       '00000000-0000-0000-0000-000000000062'));

DELETE FROM custom_field_groups
WHERE slug IN (
    'wiki_attraction_overview', 'wiki_attraction_plan_your_visit',
    'wiki_country_attractions', 'wiki_state_attractions', 'wiki_city_attractions'
);

DELETE FROM wiki_type_parents WHERE space_type_id = '00000000-0000-0000-0000-000000000071';
DELETE FROM wiki_type_configs WHERE space_type_id = '00000000-0000-0000-0000-000000000071';
DELETE FROM space_types WHERE id = '00000000-0000-0000-0000-000000000071';

ALTER TABLE space_types DROP COLUMN wiki_only;

COMMIT;
```

> `wiki_type_categories` deletes before `custom_field_groups` — `field_group_id` is `ON DELETE RESTRICT`.

- [ ] **Step 3: Add `wiki_only` to the space_types queries**

In `internal/db/queries/spaces.sql` (lines ~20-63): append `, wiki_only` to the column list of `ListSpaceTypes`, `ListSpaceTypesFiltered`, `GetSpaceTypeByID`, `GetSpaceTypeBySlug`, and both `RETURNING` lists. For `UpdateSpaceType`, add `wiki_only = sqlc.narg(wiki_only)` to the SET clause.

- [ ] **Step 4: Apply + regenerate**

Run: `make migrate` (requires local PG; if unavailable, skip to regen and note it)
Run: `make sqlc-gen`
Expected: `gen.SpaceType` gains `WikiOnly bool`; `gen.UpdateSpaceTypeParams` gains `WikiOnly pgtype.Bool`.

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: PASS (any other explicit space_types column lists elsewhere — grep `FROM space_types` — that missed `wiki_only` only narrows those row structs, not breaks them).

- [ ] **Step 6: Commit**

```bash
git add migrations/000795_space_types_wiki_only_attractions.*.sql internal/db/queries/spaces.sql internal/db/gen/
git commit -m "feat(wiki): space_types.wiki_only + attraction type/category seeds (§80)"
```

---

### Task 2: Wiki-only gating — axis coercion + write rejection + DTO flag

**Files:**
- Modify: `internal/spaces/spaces.go` (wiki_only axes + `ErrWikiOnlySpace` + `IsWikiOnlyType`)
- Modify: `internal/spaces/permissions.go` (`CanPostInSpace` early deny)
- Modify: `internal/spaces/space_crud_handlers.go` (coercion in `CreateSpace`/`UpdateSpace`; `wiki_only` in `spacesWithDetails`)
- Modify: `internal/spaces/space_membership_handlers.go` (join 409)
- Modify: `internal/spaces/space_rules_handlers.go` (rules 409)
- Modify: `internal/content/handlers.go` (~line 1028-1040: map wiki-only error → 409)
- Modify: `internal/db/queries/spaces.sql` (new `GetSpaceTypeWikiOnly`? — no, reuse `GetSpaceTypeByID`, already returns full row)
- Test: `internal/spaces/spaces_test.go` (or new `wiki_only_test.go`)

**Interfaces:**
- Produces:
  - `spaces.ErrWikiOnlySpace` — error value carrying marker method `WikiOnlySpace() bool`
  - `func (s *Service) IsWikiOnlyType(ctx context.Context, spaceTypeID pgtype.UUID) bool`
  - `func WikiOnlyPermAxes() SpacePermAxes`
  - space list/detail maps gain `"wiki_only": bool`
- Consumes: `gen.SpaceType.WikiOnly` (Task 1)

- [ ] **Step 1: Write the failing tests**

New file `internal/spaces/wiki_only_test.go` — pure unit tests (no DB; `IsWikiOnlyType`/`CanPostInSpace` need a store, so they get covered by the handler-level/integration path rather than fabricated rows):

```go
package spaces

import (
	"errors"
	"testing"
)

func TestWikiOnlyPermAxes(t *testing.T) {
	a := WikiOnlyPermAxes()
	if a.Visibility != VisibilityPublic ||
		a.Posting != PermAdmins || a.Replying != PermAdmins || a.Commenting != PermAdmins ||
		a.Joining != JoiningInvite {
		t.Fatalf("unexpected wiki-only axes: %+v", a)
	}
}

func TestErrWikiOnlySpace_Marker(t *testing.T) {
	var m interface{ WikiOnlySpace() bool }
	if !errors.As(ErrWikiOnlySpace, &m) {
		t.Fatal("ErrWikiOnlySpace must satisfy the WikiOnlySpace marker")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/spaces/ -run WikiOnly -v`
Expected: FAIL — `WikiOnlyPermAxes`/`ErrWikiOnlySpace` undefined.

- [ ] **Step 3: Implement the spaces-package core**

In `internal/spaces/spaces.go` after `DefaultPermAxes`:

```go
// ErrWikiOnlySpace rejects writes into a space whose type is wiki-only
// (§80 Wiki-Only Space Types). The marker method lets consumers in
// other packages detect it without importing this package.
var ErrWikiOnlySpace = wikiOnlySpaceError{errors.New("wiki_only_space")}

type wikiOnlySpaceError struct{ error }

func (wikiOnlySpaceError) WikiOnlySpace() bool { return true }

// WikiOnlyPermAxes returns the forced axes for wiki-only space types:
// public, admin-only write surface, invite-only join (§80).
func WikiOnlyPermAxes() SpacePermAxes {
	return SpacePermAxes{
		Visibility: VisibilityPublic,
		Posting:    PermAdmins,
		Replying:   PermAdmins,
		Commenting: PermAdmins,
		Joining:    JoiningInvite,
	}
}

// IsWikiOnlyType reports whether the given space type is wiki-only.
// Fails closed (false) so a lookup error never converts a community
// space into a wiki-only one.
func (s *Service) IsWikiOnlyType(ctx context.Context, spaceTypeID pgtype.UUID) bool {
	st, err := s.read.GetSpaceTypeByID(ctx, spaceTypeID)
	if err != nil || !st.WikiOnly {
		return false
	}
	return true
}
```

In `internal/spaces/permissions.go` at the top of `CanPostInSpace` (after the space is available — its `SpaceTypeID` field):

```go
if s.IsWikiOnlyType(ctx, space.SpaceTypeID) {
	return false // wiki-only spaces accept no posts (§80)
}
```

- [ ] **Step 4: Coerce axes in the service layer (not the handlers)**

Coercion belongs in `Service.CreateSpaceWithOwner` and `Service.UpdateSpace` (`internal/spaces/store.go`) so every write path — HTTP create/update, the attraction seeder, admin imports — gets the forced axes without relying on the caller:

```go
// §80 wiki-only types: force the restrictive axes regardless of input.
if s.IsWikiOnlyType(ctx, params.SpaceTypeID) {
	params.Visibility, params.PostingPerm, params.ReplyingPerm,
		params.CommentingPerm, params.JoiningPerm =
		VisibilityPublic, PermAdmins, PermAdmins, PermAdmins, JoiningInvite
}
```

In `UpdateSpace`, coerce via `existing.SpaceTypeID` (the type can't change on update — `UpdateSpaceParams` has no space_type_id column). Handlers need no wiki-only branch; the service guarantees it. The seeder (Task 6) passes `WikiOnlyPermAxes()` values anyway for readability.

- [ ] **Step 5: Reject join + rules writes with 409**

In `space_membership_handlers.go` `ToggleSpaceMembership`, immediately after `spaceFromContext`:

```go
if h.Service.IsWikiOnlyType(r.Context(), space.SpaceTypeID) {
	errpkg.WriteError(w, http.StatusConflict, "wiki_only_space",
		"This space is wiki-only")
	return
}
```

Same block at the top of `CreateSpaceRule` in `space_rules_handlers.go` (it also resolves the space from context; add after resolution).

- [ ] **Step 6: Map the sentinel in content creation**

In `internal/spaces/adapter.go` `PostCheckerAdapter.CanPostInSpace`: **load the space and check `IsWikiOnlyType` BEFORE the super-admin bypass** — wiki-only is a structural property of the space type, not a permission the admin override may skip:

```go
space, err := a.Service.GetSpace(ctx, spaceID)
if err != nil {
	return false, fmt.Errorf("loading space for post check: %w", err)
}
if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
	return false, ErrWikiOnlySpace
}
// Super admins bypass all posting-perm checks (existing block follows).
```

In `internal/content/handlers.go` (~line 1029) replace the error/`!allowed` block:

```go
allowed, err := h.spaceChecker.CanPostInSpace(r.Context(), createInput.SpaceID, user.UserID)
if err != nil {
	var wikiOnly interface{ WikiOnlySpace() bool }
	if errors.As(err, &wikiOnly) {
		errpkg.WriteError(w, http.StatusConflict, "wiki_only_space",
			"This space is wiki-only")
		return
	}
	errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check space posting permission")
	return
}
```

- [ ] **Step 7: `wiki_only` on space responses**

`wiki_only` is a **top-level key on the response `data` map**, not a field inside the space row (the row serializes verbatim and the column lives on `space_types`):

- `GetSpace` detail map (`space_crud_handlers.go` ~line 644): add `"wiki_only": h.Service.IsWikiOnlyType(r.Context(), space.SpaceTypeID)` beside `country_currency_code` — the wiki index reads `data.spaceDetail.wiki_only`.
- `AdminGetSpace` detail map (~line 686): same key.
- `spacesWithDetails` (~line 199): build a `typeID → wiki_only` map once via `h.Service.ListSpaceTypes(ctx)` (small table, already a service method), then set `m["wiki_only"] = flag` on each row map, keyed on the row's `SpaceTypeID` reflection field (same pattern used for `LogoMediaID`).

`/space-types` picks up `wiki_only` on the type row automatically once the column exists (gen struct json tags) — the `SpaceType` TS interface gains `wiki_only?: boolean`.

- [ ] **Step 8: Test + build**

Run: `go test ./internal/spaces/ ./internal/content/ -run 'WikiOnly|CanPost' -v && go build ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/spaces/ internal/content/handlers.go
git commit -m "feat(wiki): wiki_only space gating — axis coercion + write rejection (§80)"
```

---

### Task 3: Rich directory cards — image_url + blurb + kind on children

**Files:**
- Modify: `internal/db/queries/wiki_types.sql` (`ListWikiDirectoryChildren` — add `logo_media_id`, `header_image_id`, `description`, `wiki_content`)
- Modify: `internal/wiki/store.go:1511-1569` (`WikiDirectoryChild` fields + mapping)
- Modify: `internal/wiki/service_hierarchy.go:133-247` (DTO + `directoryChildRows` → enriched rows)
- Modify: `internal/wiki/service.go` (new `resolveImageURL` for 640×360 covers; kind cfv batch in `computeDirectory`)
- Test: `internal/wiki/service_hierarchy_test.go`

**Interfaces:**
- Consumes: `s.media` (`mediaURLResolver`, already wired), `gen.ListCustomFieldValuesWithDefsForEntities` (`entity_type='space'`, `entity_ids`)
- Produces: `WikiDirectoryChildDTO{ImageURL, Blurb, Kind}`; `directoryChildRows(children, limit, enrich func(*WikiDirectoryChildDTO))`

- [ ] **Step 1: Write the failing test**

In `service_hierarchy_test.go` — exercise the pure mapping:

```go
func TestDirectoryChildRows_Enrichment(t *testing.T) {
	children := []WikiDirectoryChild{{
		Slug: "olumo-rock", Name: "Olumo Rock", Level: "custom",
		HeaderImageID: pgtype.UUID{Valid: true}, Description: strPtr("Ancient rock fortress"),
	}}
	rows := directoryChildRows(children, 0, func(d *WikiDirectoryChildDTO, c *WikiDirectoryChild) {
		d.ImageURL = "signed:" + c.HeaderImageID.String()
		d.Blurb = dirBlurb(c)
	})
	if rows[0].ImageURL == "" || rows[0].Blurb == "" {
		t.Fatalf("expected enriched card, got %+v", rows[0])
	}
}

func TestDirBlurb_ExtractFallback(t *testing.T) {
	c := &WikiDirectoryChild{WikiContent: []byte(`{"extract":"A monolith in Abeokuta, Nigeria. Second sentence."}`)}
	if got := dirBlurb(c); got == "" || len(got) > 160 {
		t.Fatalf("blurb: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/wiki/ -run 'DirectoryChildRows|DirBlurb' -v`
Expected: FAIL — signatures don't exist yet.

- [ ] **Step 3: Extend the SQL + store**

`internal/db/queries/wiki_types.sql` — in `ListWikiDirectoryChildren` SELECT list add `s.logo_media_id, s.header_image_id, s.description, s.wiki_content`. They need no `GROUP BY` addition — `s.id` is the `spaces` PK already in GROUP BY, so Postgres functional dependency covers sibling columns. `WikiSpaceHasDirectoryChildren` is unchanged.

`internal/wiki/store.go`:

```go
type WikiDirectoryChild struct {
	ID            pgtype.UUID
	Slug          string
	Name          string
	Level         string
	SpaceTypeID   pgtype.UUID
	SpaceTypeSlug string
	Depth         int32
	GeoCode       *string
	PageCount     int64
	LogoMediaID   pgtype.UUID
	HeaderImageID pgtype.UUID
	Description   *string
	WikiContent   []byte
}
```

Map the new row fields in `ListDirectoryChildren`. Regenerate: `make sqlc-gen`.

- [ ] **Step 4: Enrich the DTO**

`service_hierarchy.go`:

```go
type WikiDirectoryChildDTO struct {
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	Level     string  `json:"level"`
	TypeSlug  string  `json:"type_slug,omitempty"`
	GeoCode   *string `json:"geo_code,omitempty"`
	PageCount int64   `json:"page_count"`
	Depth     int32   `json:"depth"`
	ImageURL  string  `json:"image_url,omitempty"`
	Blurb     string  `json:"blurb,omitempty"`
	Kind      string  `json:"kind,omitempty"`
}
```

`directoryChildRows` gains an optional `enrich func(*WikiDirectoryChildDTO, *WikiDirectoryChild)` applied per row. `computeDirectory` passes a closure that:

```go
enrich := func(d *WikiDirectoryChildDTO, c *WikiDirectoryChild) {
	d.ImageURL = s.resolveCardImageURL(ctx, c)   // header preferred, logo fallback, 640×360 webp
	d.Blurb = dirBlurb(c)                        // description, else ~140-char wiki extract
	d.Kind = kinds[c.ID.String()]                // from the batched kind cfv read
}
```

`resolveCardImageURL` (service.go, beside `resolveLogoURL`):

```go
func (s *Service) resolveCardImageURL(ctx context.Context, c *WikiDirectoryChild) string {
	if s.media == nil {
		return ""
	}
	id := c.HeaderImageID
	if !id.Valid {
		id = c.LogoMediaID
	}
	if !id.Valid {
		return ""
	}
	m, err := s.media.GetMedia(ctx, id)
	if err != nil {
		return ""
	}
	return s.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 640, Height: 360, Format: "webp"})
}
```

`dirBlurb` (service_hierarchy.go):

```go
// dirBlurb picks the card subtitle: stored description first, else the
// first ~140 chars of the auto-populated Wikipedia extract.
func dirBlurb(c *WikiDirectoryChild) string {
	if c.Description != nil && strings.TrimSpace(*c.Description) != "" {
		return strings.TrimSpace(*c.Description)
	}
	var wc struct {
		Extract string `json:"extract"`
	}
	if len(c.WikiContent) > 0 && json.Unmarshal(c.WikiContent, &wc) == nil {
		ex := strings.TrimSpace(wc.Extract)
		if len(ex) > 140 {
			if cut := strings.LastIndex(ex[:140], " "); cut > 60 {
				return ex[:cut] + "…"
			}
			return ex[:140] + "…"
		}
		return ex
	}
	return ""
}
```

**Kind batch read** in `computeDirectory` before building rows — collect child IDs, call `s.fields.ListFieldValuesWithDefsForEntities(ctx, "space", ids)` once (the `fieldReadStore` interface already exposes it), filter rows to `FieldKey == "kind"`, and build a `entity_id → kind` map. `select` stores `{"value": "museum"}` in `ValueJson` — unmarshal that shape:

```go
func cfvSelectValue(raw []byte) string {
	var v struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v.Value
}
```

Nil-safe: `s.fields == nil` → kinds empty. The batched read returns all visible values for those spaces — one query regardless of grid size (only run it when ≥1 child's `SpaceTypeSlug == "attraction"` to keep non-attraction directories at their current query count).

- [ ] **Step 5: Test + build**

Run: `make sqlc-gen && go test ./internal/wiki/ -run 'DirectoryChildRows|DirBlurb' -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/wiki_types.sql internal/db/gen/ internal/wiki/
git commit -m "feat(wiki): image_url/blurb/kind on directory-mode children (§80)"
```

---

### Task 4: Media field values — image_* fields resolve to {id,url,alt}

**Files:**
- Modify: `internal/wiki/service_fields.go` (`cfvValueJSON` call sites ~lines 522, 710 → media-aware variant)
- Test: `internal/wiki/service_fields_test.go` (exists? else `service_hierarchy_test.go`)

**Interfaces:**
- Produces: `func (s *Service) cfvDisplayValue(ctx context.Context, fieldType string, v gen.ListCustomFieldValuesWithDefsByEntityRow) json.RawMessage` — for `image_single`/`image_multi`/`media` returns `{"items":[{"id","url","alt","full_url"}]}`; all other types delegate to `cfvValueJSON`.
- Consumes: `s.media` (mediaURLResolver)

- [ ] **Step 1: Write the failing test**

```go
func TestCFVDisplayValue_ImageMulti(t *testing.T) {
	svc := &Service{media: fakeMediaResolver{url: "img/"}}
	v := gen.ListCustomFieldValuesWithDefsByEntityRow{
		FieldType: customfields.FieldTypeImageMulti,
		ValueJson: []byte(`{"media_ids":["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000002"]}`),
	}
	out := svc.cfvDisplayValue(context.Background(), v)
	var got struct {
		Items []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil || len(got.Items) != 2 {
		t.Fatalf("media value: %s err=%v", out, err)
	}
}
```

(`fakeMediaResolver` — a test double satisfying `mediaURLResolver`: `GetMedia` returns a `gen.Medium{AltText: …}`, `GetMediaURL` returns `"img/"+id`.)

- [ ] **Step 2: Run to verify failure** — undefined method/type.

- [ ] **Step 3: Implement**

In `service_fields.go`:

```go
// cfvDisplayValue renders a stored cfv row for API reads. Image/media
// types resolve their stored media_ids to {id,url,alt,full_url} items via
// the media resolver (§80 attractions: gallery rendering); all other
// types keep the verbatim value_json.
func (s *Service) cfvDisplayValue(ctx context.Context, v gen.ListCustomFieldValuesWithDefsByEntityRow) json.RawMessage {
	switch v.FieldType {
	case customfields.FieldTypeImageSingle, customfields.FieldTypeImageMulti, customfields.FieldTypeMedia:
		return s.resolveMediaValue(ctx, v.ValueJson)
	default:
		return cfvValueJSON(v)
	}
}

func (s *Service) resolveMediaValue(ctx context.Context, raw []byte) json.RawMessage {
	if len(raw) == 0 || s.media == nil {
		return nil
	}
	var stored struct {
		MediaIDs []string `json:"media_ids"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil || len(stored.MediaIDs) == 0 {
		return json.RawMessage(raw)
	}
	type item struct {
		ID      string `json:"id"`
		URL     string `json:"url"`
		Alt     string `json:"alt,omitempty"`
		FullURL string `json:"full_url,omitempty"`
	}
	items := make([]item, 0, len(stored.MediaIDs))
	for _, idStr := range stored.MediaIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil || !id.Valid {
			continue
		}
		m, err := s.media.GetMedia(ctx, id)
		if err != nil {
			continue
		}
		it := item{
			ID:      idStr,
			URL:     s.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 320, Height: 240, Format: "webp"}),
			FullURL: s.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 1600, Format: "webp"}),
		}
		if m.AltText != nil {
			it.Alt = *m.AltText
		}
		items = append(items, it)
	}
	out, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return json.RawMessage(raw)
	}
	return out
}
```

Call sites: replace `dto.Value = cfvValueJSON(v)` at ~line 522 and ~line 710 with `dto.Value = s.cfvDisplayValue(ctx, v)`. Check the row types — both call sites take `gen.ListCustomFieldValuesWithDefsByEntityRow`; if the second site uses a different row type (e.g. `…ForEntitiesRow`), give `cfvDisplayValue` two thin wrappers or take `(fieldType string, valueJson []byte)`.

- [ ] **Step 4: Test + build** — `go test ./internal/wiki/ -run CFVDisplayValue -v && go build ./...`
- [ ] **Step 5: Commit** — `git commit -m "feat(wiki): resolve image/media field values to signed URLs (§80)"`

---

### Task 5: Attraction auto-populate (Wikipedia + Wikidata enrichment)

**Files:**
- Modify: `internal/spaces/processoradapter.go` (`AutoPopulateSpace` switch line 231, new `populateAttractionSpace`, attraction keyword set)
- Test: `internal/spaces/processoradapter_test.go` (exists? else skip — the geo tests establish the pattern)

**Interfaces:**
- Consumes: existing `fetchWikipediaSummary`, `searchWikidataEntities`, `fetchWikidataEntity`, `storeSpaceImage`, `enrichUniversityFields` pattern via `a.customFields.UpsertFieldValue`
- Produces: `case "attraction"` in `AutoPopulateSpace`; `attractionTypeKeywords`; `(*ProcessorAdapter).populateAttractionSpace`

- [ ] **Step 1: Write the failing test** — pure unit test on the keyword scorer:

```go
func TestAttractionTypeKeywords_Score(t *testing.T) {
	hit := wikidataSearchResult{ID: "Q1", Description: "waterfall in Zambia and Zimbabwe"}
	if scoreAttractionHit(hit, "livingstone") <= 0 {
		t.Fatal("waterfall description must score positively")
	}
}
```

- [ ] **Step 2: Implement**

In `AutoPopulateSpace`'s switch (line ~231):

```go
case "attraction":
	populateErr = a.populateAttractionSpace(ctx, &space)
```

Add the keyword set to `geoTypeKeywords` (the amendment's shape — same scorer reused; `geoWikidataEntity`'s `keywords := geoTypeKeywords[spaceType]` lookup needs no other change):

```go
"attraction": {"attraction", "museum", "landmark", "monument", "memorial",
	"park", "garden", "castle", "fort", "palace", "mosque", "church",
	"cathedral", "temple", "shrine", "market", "zoo", "aquarium",
	"waterfall", "mountain", "hill", "rock", "monolith", "inselberg",
	"lake", "river", "cave", "beach", "forest", "reserve", "canyon",
	"spring", "falls", "historic site", "heritage"},
```

Add `Coords string` to `wikidataUniversity` + P625 claim parsing in `fetchWikidataEntity` (globecoordinate value):

```go
// P625 = coordinate location ({ "latitude": -17.92, "longitude": 25.86, ... })
if claims, ok := entity.Claims["P625"]; ok && len(claims) > 0 {
	var coordObj struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if err := json.Unmarshal(claims[0].Mainsnak.Datavalue.Value, &coordObj); err == nil { // #nosec G104 -- best-effort Wikidata parse
		uni.Coords = fmt.Sprintf("%g,%g", coordObj.Latitude, coordObj.Longitude)
	}
}
```

```go
// attractionFieldUUIDs are the deterministic custom_fields IDs seeded by
// migration 000795 (wiki_attraction_overview group) — same convention as
// universityFieldUUIDs.
var attractionFieldUUIDs = struct {
	Coords      string
	Website     string
	WikidataID  string
}{
	Coords:     "00000000-0000-0000-0000-000000000113",
	Website:    "00000000-0000-0000-0000-000000000116",
	WikidataID: "00000000-0000-0000-0000-000000000117",
}

// populateAttractionSpace enriches an attraction space from Wikipedia +
// Wikidata (§80 Wiki-Only Space Types): summary → wiki_content, P18 →
// header, P154/P94 → logo, official website + P625 coordinates → cfvs.
//
// The seeder stamps wiki_content with {"wikidata_id": "Q…"} at creation —
// when present the entity is fetched DIRECTLY by QID (deterministic: no
// scored search, no imagery gate). The qid is then merged back into the
// stored summary JSON so downstream dedup stays valid.
func (a *ProcessorAdapter) populateAttractionSpace(ctx context.Context, space *gen.GetSpaceByIDRow) error {
	var wikiErr error
	var wikiSummary *wikipediaSummary

	wikiSummary, err := fetchWikipediaSummary(ctx, space.Name)
	switch {
	case err == nil:
		// Preserve the seeded wikidata_id across the overwrite — merge it
		// into the summary JSON before persisting.
		var merged map[string]any
		if merr := json.Unmarshal(mustMarshal(wikiSummary), &merged); merr == nil {
			if qid := seededWikidataID(space.WikiContent); qid != "" {
				merged["wikidata_id"] = qid
			}
			if wikiJSON, jerr := json.Marshal(merged); jerr == nil {
				if _, uerr := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); uerr != nil {
					slog.Warn("space:auto_populate — wiki content update failed", "space_id", space.ID.String(), "error", uerr)
				}
			}
		}
		if !space.HeaderImageID.Valid {
			a.storeSpaceImage(ctx, space, firstNonEmpty(wikiSummary.OriginalImage, wikiSummary.Thumbnail), "header")
		}
	case errors.Is(err, ErrWikiNotFound):
		slog.Warn("space:auto_populate — no Wikipedia article for attraction", "space_id", space.ID.String(), "name", space.Name)
	default:
		wikiErr = fmt.Errorf("fetching Wikipedia summary for %q: %w", space.Name, err)
	}

	ent := a.attractionWikidataEntity(ctx, space)
	if ent != nil {
		if !space.LogoMediaID.Valid {
			a.storeSpaceImage(ctx, space, firstNonEmpty(ent.Logo, ent.CoatOfArms), "logo")
		}
		if !space.HeaderImageID.Valid {
			a.storeSpaceImage(ctx, space, ent.Image, "header")
		}
		a.enrichAttractionFields(ctx, space, ent)
	} else if qid := seededWikidataID(space.WikiContent); qid != "" {
		// No resolved entity but the seed stamped a QID — still record it so
		// dedup and the overview field stay populated.
		a.upsertAttractionTextField(ctx, space, attractionFieldUUIDs.WikidataID, qid)
	}

	return wikiErr
}

// seededWikidataID reads the qid the seeder stamped into wiki_content
// ({"wikidata_id": "Q…"}); "" for admin-created attractions.
func seededWikidataID(wikiContent []byte) string {
	var wc struct {
		WikidataID string `json:"wikidata_id"`
	}
	if err := json.Unmarshal(wikiContent, &wc); err != nil {
		return ""
	}
	return strings.TrimSpace(wc.WikidataID)
}

// attractionWikidataEntity resolves the Wikidata entity for an attraction:
// the seeded QID (direct fetch — deterministic) when present, else the
// scored search used by geo spaces. Unlike geoWikidataEntity the acceptance
// gate is any usable claim (image, website, coords) — POI enrichment is
// still valuable when the entity carries no image.
func (a *ProcessorAdapter) attractionWikidataEntity(ctx context.Context, space *gen.GetSpaceByIDRow) *wikidataUniversity {
	if qid := seededWikidataID(space.WikiContent); qid != "" {
		if ent, err := fetchWikidataEntity(ctx, qid); err == nil {
			return ent
		}
	}
	ent := a.geoWikidataEntity(ctx, space, "attraction")
	if ent == nil {
		return nil
	}
	if ent.Image != "" || ent.Logo != "" || ent.Website != "" || ent.Coords != "" {
		return ent
	}
	return nil
}
```

> Note: `geoWikidataEntity`'s existing gate (`Flag||CoatOfArms||Image`) already covers the common case (P18 image); `attractionWikidataEntity` wraps it for the QID-first path and the relaxed acceptance. If the wrap gets awkward, parameterize `geoWikidataEntity`'s accept check with a `func(*wikidataUniversity) bool` — geo callers pass the existing imagery predicate.
>
> `enrichAttractionFields` mirrors `enrichUniversityFields`: `UpsertFieldValue` (ValueText) for `website` (ent.Website), `coordinates` (ent.Coords), `wikidata_id` (ent.WikidataID) via `attractionFieldUUIDs`; nil-guard on `a.customFields`; best-effort logging, never returned errors.
>
> `mustMarshal` — existing codebase helper or inline `b, _ := json.Marshal(...)`; check for an existing helper before adding one.

- [ ] **Step 3: Test + build** — `go test ./internal/spaces/ -run Attraction -v && go build ./...`
- [ ] **Step 4: Commit** — `git commit -m "feat(spaces): attraction auto-populate via Wikipedia/Wikidata (§80)"`

---

### Task 6: `wiki:seed_attractions` task + admin trigger route

**Files:**
- Modify: `internal/worker/tasks.go` (`TypeWikiSeedAttractions` const + `{TypeWikiSeedAttractions, HandleWikiSeedAttractions}` registry entry)
- Modify: `internal/worker/enqueuer.go` (low-tier `taskTier` entry + `taskTypeCatalog` entry)
- Modify: `internal/worker/handlers.go` (`HandleWikiSeedAttractions`, mirroring `HandleWikiSeedStubPages` ~line 5560)
- Create: `internal/worker/wikiseedattractionsprocessor.go` (interface + package var + `Register…`, mirroring `wikiseedprocessor.go`)
- Modify: `internal/worker/noop.go` (nil-guard registration, mirroring `noopSpaceAutoPopulateProcessor` ~line 104)
- Create: `internal/spaces/attraction_seeder.go` (SPARQL candidates + create/link/enqueue on `*ProcessorAdapter`)
- Create: `internal/spaces/attraction_seed_handler.go` (`HandleSeedAttractions` on `*Handlers` + `SetAttractionSeeder` with a same-package interface)
- Modify: `internal/db/queries/spaces.sql` (`SpaceExistsByWikidataID` dedup query) + `make sqlc-gen`
- Modify: `internal/server/server.go` (route registration + processor + seeder wiring)

**Interfaces:**
- Produces:
  - `worker.TypeWikiSeedAttractions = "wiki:seed_attractions"`
  - `worker.WikiSeedAttractionsProcessor interface { SeedAttractions(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error }` — implemented by `*spaces.ProcessorAdapter` (it already owns `svc`, `systemUserID`, `mediaUploader`, `customFields`, and sits beside `fetchWikidataEntity`/`wikiRateLimiter`)
  - `func (a *ProcessorAdapter) ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]AttractionCandidate, error)`
  - `spaces.attractionSeeder` interface (`ListAttractionCandidates` only) injected into `spaces.Handlers` via `SetAttractionSeeder` — same package, no wiki↔spaces coupling
  - `POST /api/v1/admin/wiki/seed/attractions {within, limit?, dry_run?}` (`wiki.manage`) — served by `spacesHandlers.HandleSeedAttractions` (route path ≠ package)
- `AttractionCandidate{QID, Name, Description}`; `AttractionSeedResult{Candidates, Created, SkippedDuplicate, Failed int, Slugs []string}`

- [ ] **Step 1: Write the failing test** — pure unit: slug dedup + dry-run no-writes. Seed the seeder with a fake candidate source:

```go
func TestSeedAttractions_DryRunNoWrites(t *testing.T) {
	created := 0
	svc := &Service{ /* injected candidateSource returning 2 items, create hook counting writes */ }
	res, err := svc.SeedAttractions(ctx, parentID, 0, true)
	// dry_run: res.Created == 0, candidates reported, created == 0
}
```

Structure the seeder so candidate-fetch, dedup-lookup, space-create, and enqueue are small injected funcs (the Service wires the real ones; tests inject fakes — same pattern the codebase uses for adapters).

- [ ] **Step 2: Implement the seeder** (`internal/spaces/attraction_seeder.go`)

```go
// attraction_seeder.go — §80 Wiki-Only Space Types: Wikidata bulk seeding
// of attraction spaces under a geo parent (wiki:seed_attractions).
package spaces

// attractionClasses — Wikidata P31 classes that qualify as attractions.
var attractionClasses = []string{
	"Q570116",   // tourist attraction
	"Q33506",    // museum
	"Q4989906",  // monument
	"Q839954",   // archaeological site
	"Q35127",    // castle... (finalize list from spec §80: waterfall Q34038,
	// mountain Q8502, rock formation Q1407350, monolith Q11194,
	// lake Q23397, cave Q35509, national park Q46169, garden Q1107656,
	// zoo Q43501, amusement park Q194195, beach Q40080, historic site,
	// religious site Q1370598…)
}

type AttractionCandidate struct {
	QID         string
	Name        string
	Description string
}

type AttractionSeedResult struct {
	Candidates       int      `json:"candidates"`
	Created          int      `json:"created"`
	SkippedDuplicate int      `json:"skipped_duplicate"`
	Failed           int      `json:"failed"`
	Slugs            []string `json:"slugs"`
}
```

- `attractionSPARQL(parentQID)` builds the query: `?item wdt:P31/wdt:P279* ?class . VALUES ?class { wd:Q… } ?item wdt:P131* wd:{parentQID} .` with `LIMIT` — executed against `https://query.wikidata.org/sparql?format=json` through `wikiRateLimiter` (same HTTP pattern as `searchWikidataEntities`; the endpoint is a fixed upstream const like the existing `https://www.wikidata.org/w/api.php` literal at processoradapter.go:862).
- Parent QID: read `space.WikiContent`'s `wikidata_id` key (geo spaces carry it — baseline seeds show `wikidata_id` in wiki_content); if absent, resolve via `searchWikidataEntity(ctx, space.Name)` with geo keyword scoring.
- **Dedup** — `SpaceExistsByWikidataID` on `internal/db/queries/spaces.sql`, checking both the cfv (authoritative) and wiki_content (belt):

```sql
-- name: SpaceExistsByWikidataID :one
-- §80 attractions: Wikidata-QID dedup for wiki:seed_attractions — the cfv
-- survives wiki_content rewrites; the JSON check covers unseeded rows.
SELECT EXISTS (
    SELECT 1 FROM custom_field_values cfv
    WHERE cfv.entity_type = 'space'
      AND cfv.field_id = '00000000-0000-0000-0000-000000000117'::uuid
      AND cfv.value_text = @qid
) OR EXISTS (
    SELECT 1 FROM spaces s
    WHERE s.deleted_at IS NULL
      AND s.wiki_content->>'wikidata_id' = @qid
);
```

- Create: `a.svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{ParentID: parentID, SpaceTypeID: <attraction id via GetSpaceTypeBySlug("attraction")>, Name, Slug, Level: "custom", Visibility: "public", PostingPerm: "admins", ReplyingPerm: "admins", CommentingPerm: "admins", JoiningPerm: "invite", Status: "active", WikiContent: {"wikidata_id": qid}}, a.systemUserID, []pgtype.UUID{parentID})` — `CreateSpaceWithOwner` links `space_parent_spaces` in the same tx; `WikiContent` is a `sqlc.narg` on `CreateSpace` so the seed stamp lands at insert. Then `upsertAttractionTextField(ctx, &space, attractionFieldUUIDs.WikidataID, qid)` + `a.svc.EnqueueAutoPopulate(ctx, space.ID)` + `a.svc.EnqueueSearchIndex(ctx, space.ID, "index")`.
- Slug collision → `-2`, `-3` suffix loop (slug is globally unique on `spaces`).
- Multi-`P131` border attractions: v1 links only the trigger parent (the SPARQL returns one row per item); multi-parent linking is a documented follow-up — `space_parent_spaces` already supports it.
- `dryRun`: returns `AttractionSeedResult{Candidates: n, Slugs: [...]}` with zero writes.
- `limit`: default 100, hard cap 500 (package const `attractionSeedMaxLimit` — same pattern as `defaultDirectoryLimit`; no new settings key).
- Result → `slog.Info` JSON summary (no audit-writer hook exists on the adapter; a `wiki_audit` row is out of scope).

- [ ] **Step 3: Wire the task**

`internal/worker/wikiseedattractionsprocessor.go`:

```go
package worker

import "context"

// WikiSeedAttractionsProcessor is the contract for wiki:seed_attractions —
// bulk-creates attraction spaces under a geo parent (§80).
type WikiSeedAttractionsProcessor interface {
	SeedAttractions(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error
}
```

`handlers.go` (near `wikiSeedStubPagesProcessor`):

```go
var wikiSeedAttractionsProcessor WikiSeedAttractionsProcessor

// RegisterWikiSeedAttractionsProcessor wires the wiki:seed_attractions task.
func RegisterWikiSeedAttractionsProcessor(p WikiSeedAttractionsProcessor) {
	wikiSeedAttractionsProcessor = p
}
```

`HandleWikiSeedAttractions` — clone `HandleWikiSeedStubPages` with payload `{parent_space_id, limit, dry_run}`; missing parent_space_id → error.

`tasks.go`: `TypeWikiSeedAttractions = "wiki:seed_attractions"` in the §80 block (near `TypeWikiSeedStubPages` ~line 133); `{TypeWikiSeedAttractions, HandleWikiSeedAttractions}` in the handler registry (near line 279).

`enqueuer.go`: `TypeWikiSeedAttractions` in the low-tier `taskTier` list (~line 281, beside `TypeSpaceAutoPopulate`) + `{TypeWikiSeedAttractions, "low", "Bulk-seed attraction spaces from Wikidata"}` in `taskTypeCatalog` (~line 489).

`noop.go`: nil-guard registration mirroring `noopSpaceAutoPopulateProcessor` (~line 104).

`internal/spaces/processoradapter.go` (or `attraction_seeder.go`): `func (a *ProcessorAdapter) SeedAttractions(ctx, parentSpaceID string, limit int, dryRun bool) error` — string→UUID scan → `a.seedAttractions(...)`. Satisfies `worker.WikiSeedAttractionsProcessor`; register `worker.RegisterWikiSeedAttractionsProcessor(spaceAutoPopulateAdapter)` at server.go ~434, and `spacesHandlers.SetAttractionSeeder(spaceAutoPopulateAdapter)` beside it.

- [ ] **Step 4: Admin route**

`internal/spaces/attraction_seed_handler.go` — the route lives on `spaces.Handlers` so the seeder interface stays same-package (wiki would need a shared `AttractionCandidate` type otherwise):

```go
// attractionSeeder is the subset of *ProcessorAdapter the seed route needs
// for dry-run previews (§80 Wiki-Only Space Types).
type attractionSeeder interface {
	ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]AttractionCandidate, error)
}

// SetAttractionSeeder wires the Wikidata candidate lister used by
// POST /admin/wiki/seed/attractions dry-run previews. Optional.
func (h *Handlers) SetAttractionSeeder(s attractionSeeder) { h.attrSeeder = s }
```

`HandleSeedAttractions`:

- Parse `{within, limit?, dry_run?}`; resolve `within` → space via `h.Service.GetSpaceBySlug(r.Context(), within)` (accepts slug or UUID per the query doc); parent must be a `country`/`state`/`city` level space → else `400 invalid_parent`.
- `dry_run: true` → synchronous `h.attrSeeder.ListAttractionCandidates(ctx, space.ID, limit)` → `200 {data:{candidates:[{qid,name,description}]}}`; nil seeder → `503 not_configured`.
- `dry_run: false` (default) → marshal `{parent_space_id, limit, dry_run:false}` → `h.Service`'s `enqueuer` (`spaceEnqueuer` is already `Enqueue(taskType, payload, opts...)`) → `202 {data:{queued:true}}`. Same-package: the handler can call `h.Service.enqueuer` directly or via a new `EnqueueTask` helper — prefer a helper `func (s *Service) EnqueueTask(taskType string, payload []byte)` if a generic one doesn't already exist (check before adding).
- Missing `within` → `400 missing_within`; unknown slug → `404 space_not_found`.

`server.go` — register inside the `wiki.manage` route block (after the types routes ~line 6690), handler on `spacesHandlers`:

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.manage")).
	Post("/admin/wiki/seed/attractions", spacesHandlers.HandleSeedAttractions)
```

- [ ] **Step 5: Build + test** — `go build ./... && go test ./internal/wiki/ ./internal/spaces/ ./internal/worker/ -v`
- [ ] **Step 6: Commit** — `git commit -m "feat(wiki): wiki:seed_attractions bulk seeder + admin trigger route (§80)"`

---

### Task 7: Frontend main — rich cards, gallery, wiki index, spaces redirect

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (`WikiDirectoryChild` + `SpaceType.wiki_only` / `Space.wiki_only`)
- Modify: `apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte` (image-first card)
- Modify: `apps/main/src/lib/components/wiki/EntityFieldsPanel.svelte` + `formatWikiFieldValue` (image rendering)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte` (gallery strip, kind chip, hide community CTA for wiki_only)
- Modify: `apps/main/src/routes/(app)/spaces/[slug]/+page.server.ts` (redirect)
- Check: media upload picker for `SuggestEdit` — **defer**: v1 galleries are seeded/admin-managed; community image uploads stay a follow-up (spec allows).

**Interfaces:**
- Consumes: `directory.children[].image_url|blurb|kind` (Task 3); `value.items[].url|alt|full_url` for image fields (Task 4); `space.wiki_only` (Task 2); `kind` cfv on the overview group.

- [ ] **Step 1: Types**

```ts
export interface WikiDirectoryChild {
	slug: string;
	name: string;
	level: string;
	type_slug?: string;
	geo_code?: string;
	page_count: number;
	depth: number;
	image_url?: string;
	blurb?: string;
	kind?: string;
}
```

`Space` interface: `wiki_only?: boolean` (list rows via `spacesWithDetails`); `SpaceType`: `wiki_only?: boolean` (free via regen); `SpaceDetail`: `wiki_only?: boolean` — **top-level**, sibling of `space` (the backend adds it to the detail `data` map, not the row).

- [ ] **Step 2: Rich directory card**

In `WikiDirectoryPanel.svelte` replace the card `<a>` block (~lines 136-151) with a two-variant render — when `child.image_url`:

```svelte
<a
	href="/wiki/{encodeURIComponent(child.slug)}"
	class="group flex flex-col overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:border-[var(--color-primary)]"
	data-sveltekit-preload-data="hover"
>
	<img src={child.image_url} alt="" loading="lazy"
		class="aspect-[16/9] w-full object-cover" />
	<span class="flex flex-col gap-1 px-4 py-3">
		<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			{child.name}
			{#if child.kind}
				<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] capitalize text-[var(--color-muted)]">{child.kind.replace('-', ' ')}</span>
			{/if}
		</span>
		{#if child.blurb}
			<span class="line-clamp-2 text-xs text-[var(--color-muted)]">{child.blurb}</span>
		{/if}
	</span>
</a>
```

Existing text card stays as the `{:else}` branch.

- [ ] **Step 3: Image field rendering**

Wherever `formatWikiFieldValue` lives (check `apps/main/src/lib/wiki/format.ts` or inline in `EntityFieldsPanel.svelte`): field `value` objects with `items[]` for `image_multi`/`image_single` field types render a thumbnail grid instead of the `<dd>` text:

```svelte
{:else if (f.field_type === 'image_multi' || f.field_type === 'image_single') && f.value?.items}
	<div class="grid grid-cols-3 gap-2 py-1.5">
		{#each f.value.items as item (item.id)}
			<a href={item.full_url || item.url} target="_blank" rel="noopener">
				<img src={item.url} alt={item.alt ?? ''} loading="lazy"
					class="aspect-[4/3] w-full rounded-[var(--radius)] object-cover" />
			</a>
		{/each}
	</div>
```

Restructure the `<dl>` row so image fields break out of the dt/dd inline pair into a full-width block (check the existing markup — line ~196-204 — and wrap: non-image fields keep the row, image fields get a block-level section).

- [ ] **Step 4: Wiki index — wiki_only CTA suppression**

`apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte`:

- `const spaceWikiOnly = $derived(data.spaceDetail?.wiki_only === true);` — **top-level** on the detail envelope (Task 2 Step 7 puts it beside `is_member`, not inside `space`).
- Hide the "Still have questions" community CTA block (~line 471) when `spaceWikiOnly` (`{#if !spaceWikiOnly}` — verify the existing `{#if}` guard wraps it).
- `WikiSidebar` gains `wikiOnly?: boolean` prop hiding the "Need Help? → Join Community" section (~line 134) — it links `/spaces/{slug}`, which now redirects back to `/wiki/{slug}` (circular dead-end). Pass `wikiOnly={spaceWikiOnly}` at **all 3 call sites**: index `+page.svelte` (~line 256), `[page_slug]/+page.svelte` (~line 289), `[page_slug]/history/+page.svelte` (~line 109). All three server loads already fetch `spaceDetail` → `wiki_only` propagates free.
- Kind chip / hero gallery strip: **defer** — `kind` already renders via `EntityFieldsPanel` as a select value; the `wiki_attraction_overview` group also feeds `WikiQuickFacts` free when the index's `quickFactsGroup` resolves to it (verify the prop's group-slug source when implementing).

- [ ] **Step 5: spaces → wiki redirect**

`apps/main/src/routes/(app)/spaces/[slug]/+page.server.ts` — after the `spaceTypes` fetch (~line 91-97):

```ts
const spaceType = spaceTypes?.find((st) => st.id === space.space_type_id);
isUniversity = spaceType?.slug === 'university';
if (spaceType?.wiki_only) {
	throw redirect(302, `/wiki/${encodeURIComponent(slug)}`);
}
```

(`redirect` from `@sveltejs/kit` — check the file's imports.)

- [ ] **Step 6: Check + commit**

Run: `cd apps/main && bunx svelte-check` (or `bun run check`)
```bash
git add apps/main/
git commit -m "feat(main): attraction cards, gallery rendering, wiki-only space UX (§80)"
```

---

### Task 8: Admin — "Seed attractions" action

**Files:**
- Modify: `apps/admin/src/lib/components/wiki/WikiTypesAdmin.svelte` (seed action — the types tab lives here, not on the page; `apps/admin/src/routes/community/wiki/+page.svelte` just hosts the tabs)

**Interfaces:**
- Consumes: `POST /api/v1/admin/wiki/seed/attractions` (Task 6)

- [ ] **Step 1: Inspect the component** — `WikiTypesAdmin.svelte` renders the wiki-types table (`space_type_slug` column ~line 240) with an `actions` array pattern.

- [ ] **Step 2: Add the seed UI** — a "Seed attractions" `ActionItem` on the types table with `show: (row) => row.space_type_slug === 'attraction'`, opening a component-local `SlideOver` (per `apps/admin/AGENTS.md` — no hand-rolled modals) containing: `within` space-slug input, `limit` number input, `dry_run` checkbox; submit → `POST /admin/wiki/seed/attractions`; on `dry_run` render the returned candidate list (name + QID) inside the slide-over; on enqueue show the 202 `queued` state via the page's existing toast/notice convention.

- [ ] **Step 3: Check + commit**

```bash
cd apps/admin && bunx svelte-check
git add apps/admin/
git commit -m "feat(admin): attraction seeding trigger on wiki types page (§80)"
```

---

## Review notes (verified against code before implementation)

- **Spec coverage:** §1 data model (Task 1) · §2 gating (Task 2 + Task 7 redirect/CTA/sidebar) · §3 rich cards (Task 3 + 7) · §4 gallery (Task 4 + 7; SuggestEdit media picker deferred) · §5 creation/seeding (Tasks 5, 6, 8) · §6 JEOS updates (done at spec time).
- **Verified anchors:** migration 000795 free; `wiki_type_categories.rollup`/`icon` exist (000750/000753); all field types valid; `fieldReadStore.ListFieldValuesWithDefsForEntities` exists; both `cfvValueJSON` call sites share `gen.ListCustomFieldValuesWithDefsByEntityRow`; `gen.Medium.AltText` is `*string`; `media.ImageProxyOpts{Width,Height,Format}`; `CreateSpaceParams.WikiContent` is `sqlc.narg` (seed stamp lands at insert); `taskTier` map + `taskTypeCatalog` + handler registry confirmed; `wiki.manage` block in server.go confirmed; `select` cfv stores `{"value":…}` in `value_json`; `/space-types` exposes `wiki_only` free via regen.
- **Corrections applied during review:**
  - Field UUIDs made deterministic (`…0111`–`…0121`) for `enrichAttractionFields`/dedup (universityFieldUUIDs convention).
  - `wiki_only` surfaces as a top-level `data` key on `GetSpace`/`AdminGetSpace` + per-row in `spacesWithDetails` — the space row itself carries no such column.
  - Seeder + `ListAttractionCandidates` live on `*ProcessorAdapter`; admin route on `spacesHandlers` (route path ≠ package).
  - Dedup checks the `wikidata_id` cfv (survives `wiki_content` rewrites) OR `wiki_content->>'wikidata_id'`; `populateAttractionSpace` preserves the seeded qid by merging it into the stored summary.
  - `PostCheckerAdapter` checks `IsWikiOnlyType` **before** the super-admin bypass.
  - Axis coercion moved into `Service.CreateSpaceWithOwner`/`UpdateSpace` so all write paths are covered.
  - `WikiSidebar.wikiOnly` prop added — Join Community links `/spaces/{slug}` which redirects back to `/wiki/{slug}` (all 3 call sites).
  - `kind` reads `value_json.value`, not `value_text`.
  - Seeder stores `{"wikidata_id": qid}` in `wiki_content` at insert AND upserts the `wikidata_id` cfv — `populateAttractionSpace` fetches that entity directly by QID (no search) for seeded spaces.
- **Deferred (documented, not blocking):** multi-`P131` border-attraction multi-parent linking (v1 links the trigger parent; the junction supports more later); SuggestEdit media picker for community gallery uploads; `kind` hero chip (renders via EntityFieldsPanel regardless).
