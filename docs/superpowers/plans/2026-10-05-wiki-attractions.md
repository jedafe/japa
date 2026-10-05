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
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, v.options, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Kind',           'kind',          'select',      '[{"value":"museum","label":"Museum"},{"value":"landmark","label":"Landmark"},{"value":"monument","label":"Monument"},{"value":"historic-site","label":"Historic Site"},{"value":"religious-site","label":"Religious Site"},{"value":"market","label":"Market"},{"value":"waterfall","label":"Waterfall"},{"value":"mountain","label":"Mountain"},{"value":"rock-formation","label":"Rock Formation"},{"value":"lake","label":"Lake"},{"value":"cave","label":"Cave"},{"value":"beach","label":"Beach"},{"value":"forest-reserve","label":"Forest Reserve"},{"value":"canyon","label":"Canyon"},{"value":"hot-spring","label":"Hot Spring"},{"value":"national-park","label":"National Park"},{"value":"garden","label":"Garden"},{"value":"zoo","label":"Zoo"},{"value":"theme-park","label":"Theme Park"},{"value":"other","label":"Other"}]'::jsonb, true,  1),
    ('Address',        'address',       'text',        NULL::jsonb, false, 2),
    ('Coordinates',    'coordinates',   'text',        NULL::jsonb, false, 3),
    ('Opening Hours',  'opening_hours', 'textarea',    NULL::jsonb, false, 4),
    ('Entry Fee',      'entry_fee',     'price_range', NULL::jsonb, false, 5),
    ('Website',        'website',       'url',         NULL::jsonb, false, 6),
    ('Wikidata ID',    'wikidata_id',   'text',        NULL::jsonb, false, 7),
    ('Gallery',        'gallery',       'image_multi', '{"max_files":12,"max_size_mb":8}'::jsonb, false, 8)
) AS v(label, key, field_type, options, filterable, position)
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

New file `internal/spaces/wiki_only_test.go` — pure unit tests (no DB; the package's existing tests fabricate rows):

```go
package spaces

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func TestWikiOnlyPermAxes(t *testing.T) {
	a := WikiOnlyPermAxes()
	if a.Visibility != VisibilityPublic ||
		a.Posting != PermAdmins || a.Replying != PermAdmins || a.Commenting != PermAdmins ||
		a.Joining != JoiningInvite {
		t.Fatalf("unexpected wiki-only axes: %+v", a)
	}
}

func TestCanPostInSpace_WikiOnlyDenied(t *testing.T) {
	svc := &Service{}
	space := gen.Space{PostingPerm: PermOpen}
	ok, err := svc.canPostInSpaceChecked(context.TODO(), space, pgtype.UUID{}, nil, nil, true)
	if err != nil || ok {
		t.Fatalf("wiki-only space must deny posting, got ok=%v err=%v", ok, err)
	}
}

func TestErrWikiOnlySpace_Marker(t *testing.T) {
	var m interface{ WikiOnlySpace() bool }
	if !errors.As(ErrWikiOnlySpace, &m) {
		t.Fatal("ErrWikiOnlySpace must satisfy the WikiOnlySpace marker")
	}
}
```

> `canPostInSpaceChecked` is the test seam introduced below: `Service.CanPostInSpace` gains a wiki-only pre-check; the seam keeps the existing signature while making the flag injectable. If simpler, have `CanPostInSpace` look up the type itself (service has `GetSpaceType`) and test only `WikiOnlyPermAxes` + the marker — the join/rules handler tests exercise the 409 via `IsWikiOnlyType`.

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

- [ ] **Step 4: Coerce axes on create/update**

In `space_crud_handlers.go` `CreateSpace` — after `axes := DefaultPermAxes()` / preset resolution and before `CreateSpace` is invoked:

```go
// §80 wiki-only types: force the restrictive axes regardless of preset.
if h.Service.IsWikiOnlyType(r.Context(), spaceTypeID) {
	axes = WikiOnlyPermAxes()
}
```

In `UpdateSpace` — after loading `existing` and before building params:

```go
posting, replying, commenting, joining := /* resolved axes */
if h.Service.IsWikiOnlyType(r.Context(), existing.SpaceTypeID) {
	posting, replying, commenting, joining =
		PermAdmins, PermAdmins, PermAdmins, JoiningInvite
}
```

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

In `internal/content/handlers.go` (~line 1029): `PostCheckerAdapter.CanPostInSpace` is changed in the spaces package to return `spaces.ErrWikiOnlySpace` when `IsWikiOnlyType` — i.e., in `internal/spaces/adapter.go` after loading `space`:

```go
if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
	return false, ErrWikiOnlySpace
}
```

Then in `content/handlers.go` replace the `!allowed`/error block:

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

In `spacesWithDetails` (`space_crud_handlers.go` ~line 196): build a `typeID → wiki_only` map once via `h.Service.ListSpaceTypes(ctx)` (small table, already a service method), then set `m["wiki_only"] = flag` on each row map, keyed on the row's `SpaceTypeID` reflection field (same pattern used for `LogoMediaID`).

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

`internal/db/queries/wiki_types.sql` — in `ListWikiDirectoryChildren` SELECT list add `s.logo_media_id, s.header_image_id, s.description, s.wiki_content`, and add them to the `GROUP BY`. Same additions to `WikiSpaceHasDirectoryChildren` are NOT needed.

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

**Kind batch read** in `computeDirectory` before building rows — collect child IDs, call `s.fields` (the fieldReadStore already injected) or a new store query wrapping `ListCustomFieldValuesWithDefsForEntities(ctx, entity_type='space', entity_ids, ` filter `field_key='kind'` in Go (the batched query returns all show_in_frontend values; filter client-side, one query total). Extract `value_text`/first select option from `value_json`. Nil-safe when `s.fields` is unset → kinds empty.

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

```go
case "attraction":
	populateErr = a.populateAttractionSpace(ctx, &space)
```

```go
// attractionTypeKeywords disambiguates Wikidata hits for POI spaces —
// mirrors geoTypeKeywords. Scores the parent's "attraction in {place}"
// description pattern.
var attractionTypeKeywords = []string{
	"attraction", "museum", "landmark", "monument", "memorial",
	"park", "garden", "castle", "fort", "palace", "mosque", "church",
	"cathedral", "temple", "shrine", "market", "zoo", "aquarium",
	"waterfall", "mountain", "hill", "rock", "monolith", "inselberg",
	"lake", "river", "cave", "beach", "forest", "reserve", "canyon",
	"spring", "falls", "historic site", "heritage",
}

// scoreAttractionHit ranks a wikidataSearchResult for an attraction:
// +2 when the description mentions the parent space name, +1 per keyword.
func scoreAttractionHit(r wikidataSearchResult, parentName string) int {
	desc := strings.ToLower(r.Description)
	s := 0
	if parentName != "" && strings.Contains(desc, parentName) {
		s += 2
	}
	for _, kw := range attractionTypeKeywords {
		if strings.Contains(desc, kw) {
			s++
			break
		}
	}
	return s
}

// populateAttractionSpace enriches an attraction space from Wikipedia +
// Wikidata (§80 Wiki-Only Space Types): summary → wiki_content, P18 →
// header, P154/P94 → logo, official website + P625 coordinates → cfvs.
func (a *ProcessorAdapter) populateAttractionSpace(ctx context.Context, space *gen.GetSpaceByIDRow) error {
	var wikiErr error

	wikiSummary, err := fetchWikipediaSummary(ctx, space.Name)
	switch {
	case err == nil:
		if wikiJSON, merr := json.Marshal(wikiSummary); merr == nil {
			if _, uerr := a.svc.UpdateSpaceWikiContent(ctx, space.ID, wikiJSON); uerr != nil {
				slog.Warn("space:auto_populate — wiki content update failed", "space_id", space.ID.String(), "error", uerr)
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

	// Wikidata entity for logo/website/coordinates — scored search,
	// first hit carrying usable claims wins (same probe cap as geo).
	ent := a.attractionWikidataEntity(ctx, space)
	if ent != nil {
		if !space.LogoMediaID.Valid {
			a.storeSpaceImage(ctx, space, firstNonEmpty(ent.Logo, ent.CoatOfArms), "logo")
		}
		if !space.HeaderImageID.Valid {
			a.storeSpaceImage(ctx, space, ent.Image, "header")
		}
		a.enrichAttractionFields(ctx, space, ent)
	}

	return wikiErr
}
```

`attractionWikidataEntity` clones `geoWikidataEntity` with `scoreAttractionHit` instead of the geo scorer (extract the shared loop or parameterize the scorer — parameterize: `func (a *ProcessorAdapter) wikidataEntity(ctx, space, scoreFn)` and make `geoWikidataEntity` a thin caller).

`enrichAttractionFields` mirrors `enrichUniversityFields`: `UpsertFieldValue` for `website` (ent.Website), `coordinates` (extend `wikidataUniversity` fetch to also pull `P625` — check `fetchWikidataEntity` claims parsing; add `Coords string` field), and `wikidata_id` (ent.WikidataID). Field IDs resolve via field key + the attraction overview group — reuse the resolution helper `enrichUniversityFields` uses (inspect it first and copy the get-or-skip pattern).

- [ ] **Step 3: Test + build** — `go test ./internal/spaces/ -run Attraction -v && go build ./...`
- [ ] **Step 4: Commit** — `git commit -m "feat(spaces): attraction auto-populate via Wikipedia/Wikidata (§80)"`

---

### Task 6: `wiki:seed_attractions` task + admin trigger route

**Files:**
- Modify: `internal/worker/tasks.go` (const + handler registry + `taskQueue` low tier + task info table entry)
- Modify: `internal/worker/handlers.go` (`HandleWikiSeedAttractions` + `RegisterWikiSeedAttractionsProcessor`)
- Modify: `internal/worker/noop.go` (noop processor guard)
- Create: `internal/worker/wikiseedattractionsprocessor.go` (interface, mirroring `spaceautopopulateprocessor.go`)
- Create: `internal/spaces/attraction_seeder.go` (SPARQL candidates + create/link/enqueue)
- Modify: `internal/wiki/handlers.go` + `handlers_types.go` (`HandleSeedAttractions`, `SetAttractionSeeder` + seeder interface)
- Modify: `internal/server/server.go` (route registration + processor + seeder wiring)

**Interfaces:**
- Produces:
  - `worker.TypeWikiSeedAttractions = "wiki:seed_attractions"`
  - `worker.WikiSeedAttractionsProcessor interface { SeedAttractions(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error }`
  - `func (s *spaces.Service) ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]AttractionCandidate, error)`
  - `func (s *spaces.Service) SeedAttractions(ctx context.Context, parentSpaceID pgtype.UUID, limit int, dryRun bool) (*AttractionSeedResult, error)`
  - `POST /api/v1/admin/wiki/seed/attractions {within, limit?, dry_run?}` (`wiki.manage`)
- `AttractionCandidate{QID, Name, Description}`; `AttractionSeedResult{Created, SkippedDuplicate, Failed int, Slugs []string}`

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

- `attractionSPARQL(parentQID)` builds the query: `?item wdt:P31/wdt:P279* ?class . VALUES ?class { wd:Q… } ?item wdt:P131* wd:{parentQID} .` with `LIMIT` — executed against `https://query.wikidata.org/sparql?format=json` through `wikiRateLimiter` (same HTTP pattern as `searchWikidataEntities`).
- Parent QID: read `spaces.wiki_content.wikidata_id`/`wikidata_id` JSON key (geo spaces store it there — the university seeds show `wikidata_id` in wiki_content); if absent, resolve via `searchWikidataEntity(ctx, space.Name)` with geo keyword scoring.
- Dedup: `wiki_content->>'wikidata_id' = qid` check via a new store query `SpaceExistsByWikidataID` (add to `internal/db/queries/spaces.sql`: `SELECT EXISTS(SELECT 1 FROM spaces WHERE wiki_content->>'wikidata_id' = @qid AND deleted_at IS NULL)`) + slug collision → `-2`, `-3` suffix loop.
- Create: `s.CreateSpace(ctx, gen.CreateSpaceParams{SpaceTypeID: <attraction type id resolved via GetSpaceTypeBySlug("attraction")>, Name, Slug, Level: "custom", Visibility: "public", PostingPerm: "admins", ReplyingPerm: "admins", CommentingPerm: "admins", JoiningPerm: "invite", Status: "active", WikiContent: {"wikidata_id": qid}})`, then `s.AddParentSpace(ctx, id, parentID)` + `s.UpdateSpaceParentID` (`parent_id` = nearest), then `s.EnqueueAutoPopulate(ctx, id)`.
- Multi-P131: keep v1 simple — link only the trigger parent; note multi-parent linking as follow-up (the query returns one row per item).
- `dryRun`: returns `AttractionSeedResult{Candidates: n}` with `Slugs` = candidate slugs, zero writes.
- Result → `audit_logs` row if an audit hook is available on the service (check `s.audit`/`auditLogger` patterns; else `slog.Info` the JSON).

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

`tasks.go`: `TypeWikiSeedAttractions = "wiki:seed_attractions"` in the §80 block; `{TypeWikiSeedAttractions, HandleWikiSeedAttractions}` in the handler registry; `{TypeWikiSeedAttractions, "low", "Bulk-seed attraction spaces from Wikidata"}` in the task-info table; add to `QueueForTask`/`taskQueue` mapping if task→queue is a separate switch (check `enqueuer.go:366`).

`noop.go`: nil-guard registration mirroring `noopSpaceAutoPopulateProcessor`.

`internal/spaces/workeradapter` or a new adapter on `Service`: `func (s *Service) SeedAttractionsJob(ctx context.Context, parentSpaceID string, limit int, dryRun bool) error` (string→UUID scan → `s.SeedAttractions`) — satisfies `worker.WikiSeedAttractionsProcessor`; register in `server.go` near `worker.RegisterSpaceAutoPopulateProcessor(...)`.

- [ ] **Step 4: Admin route**

`internal/wiki/handlers_types.go` (or a new small file): 

```go
// attractionSeeder is the subset of spaces.Service the seed route needs.
type attractionSeeder interface {
	ListAttractionCandidates(ctx context.Context, parentSpaceID pgtype.UUID, limit int) ([]spaces.AttractionCandidate, error)
}
```

> Import direction: `internal/wiki` may not import `internal/spaces` (check first — wiki already imports `internal/db/gen` and customfields; spaces imports neither wiki nor content). If importing spaces creates a cycle, define the interface against a local `AttractionCandidate` struct and adapt in server.go.

`handlers.go`:

```go
// SetAttractionSeeder wires the Wikidata candidate lister used by
// POST /admin/wiki/seed/attractions dry-run previews. Optional.
func (h *Handlers) SetAttractionSeeder(s attractionSeeder) { h.seeder = s }
```

`HandleSeedAttractions`:

- Parse `{within, limit?, dry_run?}`; resolve `within` slug → space ID via `s.service`'s existing `GetSpaceBySlug` store path (the wiki service resolves slugs for categories — reuse).
- `dry_run: true` → synchronous `ListAttractionCandidates` → `200 {candidates:[{qid,name,description}]}`.
- `dry_run: false` (default) → enqueue `wiki:seed_attractions` `{parent_space_id, limit, dry_run:false}` via `s.service.enqueuer` (`TaskEnqueuer` — already injected) → `202 {queued:true}`.
- Missing/unknown `within` → `400 invalid_space` / `404 space_not_found`.

`server.go` — register inside the `wiki.manage` route block (after the types routes):

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.manage")).
	Post("/admin/wiki/seed/attractions", wikiHandlers.HandleSeedAttractions)
```

And wiring: `wikiHandlers.SetAttractionSeeder(spacesService)` (or adapter) + `worker.RegisterWikiSeedAttractionsProcessor(...)`.

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

`Space` interface: add `wiki_only?: boolean`; `SpaceType`: add `wiki_only?: boolean`.

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

- [ ] **Step 4: Wiki index — gallery strip, kind chip, wiki_only CTA**

`apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte`:

- `const spaceWikiOnly = $derived(data.spaceDetail?.space?.wiki_only === true);`
- Hide the "Still have questions" community CTA block when `spaceWikiOnly` (`{#if !spaceWikiOnly}`).
- Kind chip: read the overview group's `kind` value — the index page's `EntityFieldsPanel` data or `data.spaceDetail` doesn't carry cfvs; simplest: render the chip from `data.spaceDetail?.space?.kind` if the backend adds it, else skip — **defer chip to the fields panel** (kind already renders there as a select value). If wanted on the hero: load `/wiki/entities/space/{id}/fields` client-side when `spaceWikiOnly` and read `kind` — acceptable but optional; mark as nice-to-have.
- Gallery strip: same source — if the fields endpoint is loaded anyway (EntityFieldsPanel does), lift the `gallery` field's `items` into a horizontal strip under the hero (`<img>` scroll-x row) when present.

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
- Modify: `apps/admin/src/routes/community/wiki/+page.svelte` (seed panel/action)

**Interfaces:**
- Consumes: `POST /api/v1/admin/wiki/seed/attractions` (Task 6)

- [ ] **Step 1: Inspect the page** — read `+page.svelte`; it's the wiki types admin. Find the wiki-types list rendering.

- [ ] **Step 2: Add the seed UI** — on the `attraction` type row (or a dedicated card on the page): a "Seed attractions" button opening the page-local SlideOver with: `within` space-slug input (text or the existing space autocomplete), `limit` number input, `dry_run` checkbox, submit → `POST /admin/wiki/seed/attractions`; on `dry_run` render the returned candidate list (name + QID) inside the slide-over. Follow the EntityPage/SlideOver conventions in `apps/admin/AGENTS.md` — no hand-rolled modals.

- [ ] **Step 3: Check + commit**

```bash
cd apps/admin && bunx svelte-check
git add apps/admin/
git commit -m "feat(admin): attraction seeding trigger on wiki types page (§80)"
```

---

## Self-Review notes

- **Spec coverage:** §1 data model (Task 1) · §2 gating (Task 2 + Task 7 redirect/CTA) · §3 rich cards (Task 3 + 7) · §4 gallery (Task 4 + 7; SuggestEdit media picker deferred — noted in Task 7) · §5 creation/seeding (Tasks 5, 6, 8) · §6 JEOS updates (done at spec time).
- **Multi-P131 linking** is spec'd as "link every resolved parent" — Task 6 v1 links the trigger parent only; border-attraction multi-linking noted as follow-up in the seeder.
- **`kind` chip on attraction hero** deferred to the fields panel (Task 7 marks it optional) — the value renders there regardless.
- **SuggestEdit media picker** (community gallery uploads) deferred to follow-up — spec called it part of §4; it's the only partial gap, flagged here so it's not lost.
