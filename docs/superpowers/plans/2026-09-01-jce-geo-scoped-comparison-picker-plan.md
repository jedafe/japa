# JCE Geo-Scoped Comparison Picker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a configurable `scope_chain` to comparison types, render it as a hierarchical filter on `/compare`, and repoint the `course` resolver to a new university-course catalog/offerings model.

**Architecture:** Extend `comparison_type_definitions.config` with `scope_chain`, add parent/region/level filters to the spaces and resolver layers, build a Svelte `ScopeChain` component for the picker, and introduce `university_course_catalog` + `university_course_offerings` tables with a dedicated store/resolver.

**Tech Stack:** Go 1.23, Chi, pgx/v5, sqlc, SvelteKit 5, TypeScript, Tailwind 4, Bun.

> **Note for this plan-update task:** no code is implemented or committed. The commit commands in the per-task steps are for the implementation phase and should not be run while verifying this plan.

## Global Constraints

- Target branch for all commits: `main`.
- Use `bun` for all JS/TS tooling (`bun install`, `bun run`, `bunx`).
- All frontend data comes from backend APIs; no hardcoded option lists.
- Do not hand-edit `internal/db/gen/`; edit `internal/db/queries/*.sql` and run `make sqlc-gen`.
- All new Valkey keys use `jc:` prefix.
- API errors use `code` (snake_case) + `message` format.
- Soft deletes only (`deleted_at`) for user-facing content.
- No AI attribution in commits, comments, or docs (per `AGENTS.md` Rule 17).
- Reference JEOS sections in code comments (`// Ref: japaconnect-engineering/sections/NN-*.md`).
- **JEOS-first:** update the relevant JEOS section/convention file before creating new tables or routes.

---

## File Structure

| File | Responsibility |
|------|----------------|
| `.jeos/sections/39-space-types-university.md` | Records `university_course_catalog` / `university_course_offerings` schema before code. |
| `.jeos/conventions/api-routes-catalog.md` | Adds `GET /spaces/regions` and `GET /spaces/subregions` to the catalog. |
| `migrations/000588_jce_scope_chain_config.up.sql` | Adds `scope_chain` to system comparison types (tenant-scoped). |
| `migrations/000588_jce_scope_chain_config.down.sql` | Removes `scope_chain` from the affected system type configs. |
| `migrations/000589_jce_university_course_indexes.up.sql` | (Reserved for any additional indexes on the new course tables if needed.) |
| `migrations/000590_jce_university_courses.up.sql` | Creates `university_course_catalog` and `university_course_offerings`. |
| `migrations/000590_jce_university_courses.down.sql` | Drops the new course tables. |
| `migrations/000591_jce_seed_demo_cities.up.sql` | Creates city spaces, `space_parent_spaces` links, and seeds `col_city` records + values for Lagos, Abuja, Accra, Nairobi. |
| `migrations/000591_jce_seed_demo_cities.down.sql` | Removes the seeded city spaces, `col_city` records, and values. |
| `migrations/000592_jce_seed_universities_courses_visas.up.sql` | Creates UCC space, links demo universities to demo cities, seeds `university_course_catalog` + `university_course_offerings`, and seeds `visa_routes` for Nigeria, Ghana, Kenya. |
| `migrations/000592_jce_seed_universities_courses_visas.down.sql` | Removes the demo UCC space, course catalog/offerings, and visa routes. |
| `internal/db/queries/spaces.sql` | New `ListSpacesByParentAndLevel`, `ListSpacesByAncestorAndType`, `CountSpacesByAncestorAndType`, `ListDistinctRegions`, `ListDistinctSubregions`; updated `ListSpacesPaginated`, `CountSpaces`, `ListSpacesByTypePaginated`, `CountSpacesByType` with `region`/`subregion`. |
| `internal/db/queries/cost_of_living.sql` | Updated `ListCOLCities` and `CountCOLCities` with `country_id`/`state_id`/`city_id` filters and `space_parent_spaces` joins. |
| `internal/db/queries/university_courses.sql` | CRUD and search for catalog/offerings. |
| `internal/spaces/store.go` | New `ListSpacesByParentAndLevel`, `ListSpacesByAncestorAndType`, `CountSpacesByAncestorAndType`, `ListDistinctRegions`, `ListDistinctSubregions`; updated `ListSpacesPaginated`, `CountSpaces`, `ListSpacesByTypePaginated`, `CountSpacesByType`. |
| `internal/spaces/space_crud_handlers.go` | Updated `ListSpaces` handler for `?parent=&level=` via `space_parent_spaces` and `?region=&subregion=&level=country`. |
| `internal/comparison/resolver/space_type.go` | Updated `Search` to use `ListSpacesByAncestorAndType` (state/city) and `region`/`subregion` (country). |
| `internal/comparison/resolver/university.go` | Updated `Search` to use `ListSpacesByAncestorAndType` with the deepest geo scope. |
| `internal/comparison/resolver/city.go` | Updated `Search` with `country_id`/`state_id`/`city_id` filters. |
| `internal/comparison/resolver/visa.go` | Updated `Search` with `country_id` filter. |
| `internal/university_courses/` | New package: `models.go`, `store.go`, `service.go`. |
| `internal/comparison/resolver/course.go` | Rewritten to resolve `university_course_offerings` (id + slug). |
| `internal/comparison/jce_service.go` | Add `ScopeChain` to `jceTypeConfig` struct. |
| `internal/server/server.go` | Register new course resolver and wire store. |
| `apps/main/src/lib/types/jce.ts` | Add `scope_chain?: string[]` to `ComparisonTypeConfig`. |
| `apps/main/src/lib/components/compare/ScopeChain.svelte` | New hierarchical filter component. |
| `apps/main/src/routes/compare/+page.server.ts` | Read scope filter params and pass to `/jce/entities/${source}`. |
| `apps/main/src/routes/compare/+page.svelte` | Render `ScopeChain`, keep selected items on filter change. |
| `apps/main/src/lib/i18n/en.json` | New scope-chain labels. |
| `tests/e2e/jce-scope-chain.spec.ts` | E2E tests for scope-chain flows. |

---

## Dependencies

- `Task 2` (spaces SQL) must be done before `Task 4` (resolvers use the SQL).
- `Task 3` (type pagination) must be done before `Task 4` (university/space-type resolvers use it).
- `Task 4` must be done before `Task 9` (frontend `/compare` uses the resolver API).
- `Task 5` (university courses tables) must be done before `Task 6` (store/service) and `Task 7` (course resolver).
- `Task 6` must be done before `Task 7` (course resolver uses the service).
- `Task 8` (`ScopeChain` component) can be done after `Task 4` and before `Task 9`.
- `Task 9` (frontend integration) must be done after `Task 8`.
- `Task 10` and `Task 10.5` (seeding) are last and depend on `Task 4`, `Task 7`, and `Task 5`.

---

## Task 1: Add `scope_chain` to system comparison types

**Files:**
- Create: `migrations/000588_jce_scope_chain_config.up.sql`
- Create: `migrations/000588_jce_scope_chain_config.down.sql`
- Test: existing `tests/e2e/jce-nested.spec.ts`, `bunx playwright test tests/e2e/jce-nested.spec.ts`

**Interfaces:**
- Produces: `comparison_type_definitions.config` with `scope_chain` for `cost_of_living`, `universities`, `countries`, `states`, `cities`, `courses`, `visa-types`.

- [ ] **Step 0: Update JEOS comparison section**

  Before changing the schema, record the new `scope_chain` field and the system-type defaults in `.jeos/sections/30-generic-comparison-system.md` under the comparison type config definition.

- [ ] **Step 1: Write migration**

```sql
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
BEGIN;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state', 'city'))
WHERE slug IN ('cost_of_living', 'universities', 'cities') AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('region', 'subregion'))
WHERE slug = 'countries' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state'))
WHERE slug = 'states' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('university'))
WHERE slug = 'courses' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country'))
WHERE slug = 'visa-types' AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
```

- [ ] **Step 2: Write down migration**

`migrations/000588_jce_scope_chain_config.down.sql`:

```sql
BEGIN;

UPDATE comparison_type_definitions
SET config = config - 'scope_chain'
WHERE slug IN ('cost_of_living', 'universities', 'countries', 'states', 'cities', 'courses', 'visa-types')
  AND is_system = true
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
```

- [ ] **Step 3: Run migration locally**

```bash
make migrate-up
```

- [ ] **Step 4: Verify config**

```bash
psql $DATABASE_URL -c "SELECT slug, config FROM comparison_type_definitions WHERE is_system = true AND tenant_id = '00000000-0000-0000-0000-000000000001';"
```

Expected: each row has `scope_chain` array.

- [ ] **Step 5: Commit**

```bash
git add migrations/000588_jce_scope_chain_config.up.sql migrations/000588_jce_scope_chain_config.down.sql .jeos/sections/30-generic-comparison-system.md
git commit -m "migrations: add scope_chain to JCE system comparison types"
```

---

## Task 2: Extend spaces SQL for `space_parent_spaces` and region/subregion filters

**Files:**
- Modify: `internal/db/queries/spaces.sql`
- Create/Modify: `internal/spaces/store.go`
- Modify: `internal/spaces/space_crud_handlers.go`
- Modify: `internal/server/server.go`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Test: `internal/spaces/store_test.go` or new `internal/spaces/spaces_test.go`

**Interfaces:**
- Produces: `ListSpacesByParentAndLevel`, `ListSpacesByAncestorAndType`, `CountSpacesByAncestorAndType`, `ListDistinctRegions`, `ListDistinctSubregions`, `ListSpacesPaginated`/`CountSpaces` with `region`/`subregion`, `ListSpacesByTypePaginated`/`CountSpacesByType` with `region`/`subregion`.

- [ ] **Step 1: Update `ListSpacesPaginated` and `CountSpaces` for `region`/`subregion`**

Replace the existing `ListSpacesPaginated` and `CountSpaces` query bodies in `internal/db/queries/spaces.sql` with:

```sql
-- name: ListSpacesPaginated :many
-- SPACES-009/011: SQL-level paginated discovery list with search, geo,
-- level, region, subregion, and sort support. Replaces the in-memory
-- filter+paginate in the ListSpaces handler. Visibility filter matches
-- ListSpacesByTenant.
-- sort accepts: '' (default → newest), 'name' (A–Z), 'members', 'followers',
-- 'posts' (all numeric DESC). Unknown values fall back to newest.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
WHERE s.tenant_id = @tenant_id AND s.deleted_at IS NULL
  AND (
    s.visibility = 'public'
    OR (
      @viewer_user_id::uuid IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM space_members sm
        WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid
          AND sm.tenant_id = s.tenant_id AND sm.status = 'approved'
      )
    )
  )
  AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
  AND (@geo_code::text = '' OR s.geo_code = @geo_code)
  AND (@level::text = '' OR s.level = @level)
  AND (@region::text = '' OR s.region = @region)
  AND (@subregion::text = '' OR s.subregion = @subregion)
ORDER BY
  CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
  CASE
    WHEN @sort::text = 'members' THEN s.member_count
    WHEN @sort::text = 'followers' THEN s.follower_count
    WHEN @sort::text = 'posts' THEN s.post_count
    ELSE NULL
  END DESC,
  s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpaces :one
-- SPACES-009: total count for the paginated discovery list.
SELECT COUNT(*) AS total
FROM spaces s
WHERE s.tenant_id = @tenant_id AND s.deleted_at IS NULL
  AND (
    s.visibility = 'public'
    OR (
      @viewer_user_id::uuid IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM space_members sm
        WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid
          AND sm.tenant_id = s.tenant_id AND sm.status = 'approved'
      )
    )
  )
  AND (@search::text = '' OR s.name ILIKE '%' || @search || '%' OR s.slug ILIKE '%' || @search || '%')
  AND (@geo_code::text = '' OR s.geo_code = @geo_code)
  AND (@level::text = '' OR s.level = @level)
  AND (@region::text = '' OR s.region = @region)
  AND (@subregion::text = '' OR s.subregion = @subregion);
```

- [ ] **Step 1.5: Update `ListChildSpaces` to use `space_parent_spaces`**

`ListChildSpaces` (used by the `?parent=` handler branch when no `level` is provided) must also traverse the canonical `space_parent_spaces` junction instead of `spaces.parent_id`:

```sql
-- name: ListChildSpaces :many
-- Returns all children of a parent space via the space_parent_spaces junction.
-- SPACES-009: uses the canonical junction so geo and custom parents are found.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
       s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
       s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
       s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.tenant_id = @tenant_id AND s.deleted_at IS NULL
  AND sps.parent_space_id = @parent_id
  AND (
    s.visibility = 'public'
    OR (
      @viewer_user_id::uuid IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM space_members sm
        WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid
          AND sm.tenant_id = s.tenant_id AND sm.status = 'approved'
      )
    )
  )
ORDER BY s.name ASC;
```

- [ ] **Step 2: Add `ListSpacesByParentAndLevel` (uses `space_parent_spaces`)

Append to `internal/db/queries/spaces.sql`:

```sql
-- name: ListSpacesByParentAndLevel :many
-- Returns spaces of a given level whose parent is linked via
-- space_parent_spaces (the canonical geo hierarchy: country → state → city).
-- Used by the JCE scope-chain picker for country/state/city traversal.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
       s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
       s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
       s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
WHERE s.tenant_id = @tenant_id AND s.deleted_at IS NULL
  AND sps.parent_space_id = @parent_id
  AND s.level = @level
  AND (
    s.visibility = 'public'
    OR (
      @viewer_user_id::uuid IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM space_members sm
        WHERE sm.space_id = s.id AND sm.user_id = @viewer_user_id::uuid
          AND sm.tenant_id = s.tenant_id AND sm.status = 'approved'
      )
    )
  )
ORDER BY s.name ASC;
```

- [ ] **Step 3: Add `ListSpacesByAncestorAndType` and `CountSpacesByAncestorAndType` (uses `space_parent_spaces`)**

Append to `internal/db/queries/spaces.sql`:

```sql
-- name: ListSpacesByAncestorAndType :many
-- Returns spaces of a given space_type that are children of an ancestor
-- space via the space_parent_spaces junction. Used by the JCE resolvers for
-- state (ancestor=country), city (ancestor=state), and university
-- (ancestor=city/state/country).
-- sort accepts: '' (newest), 'name', 'members', 'followers', 'posts'.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
       s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
       s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
       s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code,
       cfv.value_text AS initials
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
  AND cfv.entity_type = 'space'
  AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.tenant_id = @tenant_id AND s.space_type_id = @space_type_id AND s.deleted_at IS NULL
  AND sps.parent_space_id = @ancestor_space_id
  AND (@search::text = ''
    OR s.name ILIKE '%' || @search || '%'
    OR s.slug ILIKE '%' || @search || '%'
    OR cfv.value_text ILIKE '%' || @search || '%')
ORDER BY
  CASE WHEN @sort::text = 'name' THEN s.name ELSE NULL END ASC,
  CASE
    WHEN @sort::text = 'members' THEN s.member_count
    WHEN @sort::text = 'followers' THEN s.follower_count
    WHEN @sort::text = 'posts' THEN s.post_count
    ELSE NULL
  END DESC,
  s.created_at DESC, s.id DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSpacesByAncestorAndType :one
SELECT COUNT(*) AS total
FROM spaces s
JOIN space_parent_spaces sps ON sps.space_id = s.id
LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
  AND cfv.entity_type = 'space'
  AND cfv.field_id = '00000000-0000-0000-0000-000000000090'::uuid
WHERE s.tenant_id = @tenant_id AND s.space_type_id = @space_type_id AND s.deleted_at IS NULL
  AND sps.parent_space_id = @ancestor_space_id
  AND (@search::text = ''
    OR s.name ILIKE '%' || @search || '%'
    OR s.slug ILIKE '%' || @search || '%'
    OR cfv.value_text ILIKE '%' || @search || '%');
```

- [ ] **Step 4: Add region/subregion queries**

```sql
-- name: ListDistinctRegions :many
SELECT DISTINCT region
FROM spaces
WHERE tenant_id = @tenant_id AND level = 'country' AND deleted_at IS NULL
  AND region IS NOT NULL AND region <> ''
ORDER BY region ASC;

-- name: ListDistinctSubregions :many
SELECT DISTINCT subregion
FROM spaces
WHERE tenant_id = @tenant_id AND level = 'country' AND deleted_at IS NULL
  AND region = @region
  AND subregion IS NOT NULL AND subregion <> ''
ORDER BY subregion ASC;
```

- [ ] **Step 5: Regenerate sqlc**

```bash
make sqlc-gen
```

- [ ] **Step 6: Add/Update store methods**

In `internal/spaces/store.go`, update `ListSpacesPaginated` / `CountSpaces` and add the new methods. Do **not** modify `ListSpacesByTypePaginated` to add a `parent_id` filter — that is handled by `ListSpacesByAncestorAndType`.

```go
// ListSpacesPaginated returns a page of spaces with SQL-level search, geo,
// level, region, subregion, sort, and visibility filtering.
func (s *Service) ListSpacesPaginated(ctx context.Context, ID, viewerUserID pgtype.UUID, search, geoCode, level, region, subregion, sort string, limit, offset int32) ([]gen.ListSpacesPaginatedRow, error) {
    spaces, err := s.read.ListSpacesPaginated(ctx, gen.ListSpacesPaginatedParams{
        ID:     ID,
        ViewerUserID: viewerUserID,
        Search:       search,
        GeoCode:      geoCode,
        Level:        level,
        Region:       region,
        Subregion:    subregion,
        Sort:         sort,
        LimitVal:     limit,
        OffsetVal:    offset,
    })
    if err != nil {
        return nil, fmt.Errorf("listing spaces paginated: %w", err)
    }
    return spaces, nil
}

// CountSpaces returns the total count of spaces matching the paginated
// discovery filters.
func (s *Service) CountSpaces(ctx context.Context, ID, viewerUserID pgtype.UUID, search, geoCode, level, region, subregion string) (int64, error) {
    total, err := s.read.CountSpaces(ctx, gen.CountSpacesParams{
        ID:     ID,
        ViewerUserID: viewerUserID,
        Search:       search,
        GeoCode:      geoCode,
        Level:        level,
        Region:       region,
        Subregion:    subregion,
    })
    if err != nil {
        return 0, fmt.Errorf("counting spaces: %w", err)
    }
    return total, nil
}

// ListSpacesByParentAndLevel returns child spaces of a given level using the
// canonical space_parent_spaces junction.
func (s *Service) ListSpacesByParentAndLevel(ctx context.Context, ID, viewerUserID, parentID pgtype.UUID, level string) ([]gen.ListSpacesByParentAndLevelRow, error) {
    return s.read.ListSpacesByParentAndLevel(ctx, gen.ListSpacesByParentAndLevelParams{
        ID:     ID,
        ParentID:     parentID,
        Level:        level,
        ViewerUserID: viewerUserID,
    })
}

// ListSpacesByAncestorAndType returns spaces of a specific type that are linked
// to an ancestor via space_parent_spaces.
func (s *Service) ListSpacesByAncestorAndType(ctx context.Context, ID, spaceTypeID, ancestorID pgtype.UUID, search, sort string, limit, offset int32) ([]gen.ListSpacesByAncestorAndTypeRow, error) {
    return s.read.ListSpacesByAncestorAndType(ctx, gen.ListSpacesByAncestorAndTypeParams{
        ID:       ID,
        SpaceTypeID:    spaceTypeID,
        AncestorSpaceID: ancestorID,
        Search:         search,
        Sort:           sort,
        LimitVal:       limit,
        OffsetVal:      offset,
    })
}

// CountSpacesByAncestorAndType returns the count of spaces of a specific type
// linked to an ancestor via space_parent_spaces.
func (s *Service) CountSpacesByAncestorAndType(ctx context.Context, ID, spaceTypeID, ancestorID pgtype.UUID, search string) (int64, error) {
    return s.read.CountSpacesByAncestorAndType(ctx, gen.CountSpacesByAncestorAndTypeParams{
        ID:        ID,
        SpaceTypeID:     spaceTypeID,
        AncestorSpaceID: ancestorID,
        Search:          search,
    })
}

// ListDistinctRegions returns the distinct regions for country-level spaces.
func (s *Service) ListDistinctRegions(ctx context.Context, ID pgtype.UUID) ([]string, error) {
    return s.read.ListDistinctRegions(ctx, ID)
}

// ListDistinctSubregions returns the distinct subregions for a given region.
func (s *Service) ListDistinctSubregions(ctx context.Context, ID pgtype.UUID, region string) ([]string, error) {
    return s.read.ListDistinctSubregions(ctx, gen.ListDistinctSubregionsParams{
        ID: ID,
        Region:   region,
    })
}
```

- [ ] **Step 7: Update `ListSpaces` handler**

Modify `internal/spaces/space_crud_handlers.go` `ListSpaces` to:

1. Parse `?region=` and `?subregion=`.
2. Pass them to `ListSpacesPaginated` / `CountSpaces` in the default branch.
3. Use `ListSpacesByParentAndLevel` when `?parent=` and `?level=` are both present.
4. Add cases for `[]gen.ListSpacesByParentAndLevelRow` in `filterSpacesByNameSlug` and `paginateSpaces` if the in-memory branch is still used.

```go
q := r.URL.Query()
search := strings.TrimSpace(q.Get("q"))
geoCode := strings.TrimSpace(q.Get("geo"))
sort := strings.TrimSpace(q.Get("sort"))
levelFilter := strings.TrimSpace(q.Get("level"))
region := strings.TrimSpace(q.Get("region"))
subregion := strings.TrimSpace(q.Get("subregion"))
```

In the default branch:

```go
rows, perr := h.Service.ListSpacesPaginated(r.Context(), ID, viewerUserID, search, geoCode, levelFilter, region, subregion, sort, int32(limit), int32(offset))
if perr != nil {
    handleServiceError(w, perr)
    return
}
count, cerr := h.Service.CountSpaces(r.Context(), ID, viewerUserID, search, geoCode, levelFilter, region, subregion)
```

In the `?parent` branch:

```go
case q.Get("parent") != "":
    var parentID pgtype.UUID
    if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
        return
    }
    if levelFilter != "" {
        spaces, err = h.Service.ListSpacesByParentAndLevel(r.Context(), ID, viewerUserID, parentID, levelFilter)
    } else {
        spaces, err = h.Service.ListChildSpaces(r.Context(), ID, viewerUserID, parentID)
    }
```

- [ ] **Step 8: Add region/subregion endpoints**

In `internal/spaces/space_crud_handlers.go`:

```go
func (h *Handlers) ListDistinctRegions(w http.ResponseWriter, r *http.Request) {
    ID, ok := h.requireTenant(w, r)
    if !ok { return }
    regions, err := h.Service.ListDistinctRegions(r.Context(), ID)
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list regions")
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": regions})
}

func (h *Handlers) ListSubregionsByRegion(w http.ResponseWriter, r *http.Request) {
    ID, ok := h.requireTenant(w, r)
    if !ok { return }
    region := r.URL.Query().Get("region")
    if region == "" {
        errpkg.WriteError(w, http.StatusBadRequest, "missing_region", "region query parameter is required")
        return
    }
    subregions, err := h.Service.ListDistinctSubregions(r.Context(), ID, region)
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list subregions")
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subregions})
}
```

In `internal/server/server.go`, add routes **before** the `/spaces/{slug}` route so the static segments are matched first:

```go
r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.spaces.view")).Get("/spaces/regions", spaceHandlers.ListDistinctRegions)
r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.spaces.view")).Get("/spaces/subregions", spaceHandlers.ListSubregionsByRegion)
```

- [ ] **Step 9: Update the API route catalog**

In `.jeos/conventions/api-routes-catalog.md` under the **Spaces (Section 05)** section, add:

```
GET    /spaces/regions                   [public, tenant-scoped] list distinct UN regions for country-level spaces
GET    /spaces/subregions?region=        [public, tenant-scoped] list distinct subregions for a region
```

- [ ] **Step 10: Run tests**

```bash
go test ./internal/spaces/...
```

- [ ] **Step 11: Commit**

```bash
git add internal/db/queries/spaces.sql internal/spaces/store.go internal/spaces/space_crud_handlers.go .jeos/conventions/api-routes-catalog.md
git add internal/db/gen/  # generated files
make sqlc-gen && git add internal/db/gen/
git commit -m "spaces: add space_parent_spaces based hierarchy and region/subregion filters"
```

---

## Task 3: Extend `ListSpacesByTypePaginated` with `region` and `subregion` filters

**Files:**
- Modify: `internal/db/queries/spaces.sql`
- Modify: `internal/spaces/store.go`

**Interfaces:**
- Produces: `ListSpacesByTypePaginated` and `CountSpacesByType` accept optional `region` and `subregion` (no `parent_id`; use `ListSpacesByAncestorAndType` for parent/ancestor filtering).

- [ ] **Step 1: Update SQL**

In `ListSpacesByTypePaginated` and `CountSpacesByType`, add:

```sql
AND (@region::text = '' OR s.region = @region)
AND (@subregion::text = '' OR s.subregion = @subregion)
```

- [ ] **Step 2: Update store method**

```go
func (s *Service) ListSpacesByTypePaginated(ctx context.Context, ID, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error) {
    return s.read.ListSpacesByTypePaginated(ctx, gen.ListSpacesByTypePaginatedParams{
        ID:    ID,
        SpaceTypeID: spaceTypeID,
        Search:      search,
        GeoCode:     geoCode,
        Region:      region,
        Subregion:   subregion,
        Sort:        sort,
        LimitVal:    limit,
        OffsetVal:   offset,
    })
}

func (s *Service) CountSpacesByType(ctx context.Context, ID, spaceTypeID pgtype.UUID, search, geoCode, region, subregion string) (int64, error) {
    return s.read.CountSpacesByType(ctx, gen.CountSpacesByTypeParams{
        ID:    ID,
        SpaceTypeID: spaceTypeID,
        Search:      search,
        GeoCode:     geoCode,
        Region:      region,
        Subregion:   subregion,
    })
}
```

Update all existing call sites to pass empty strings for `region`/`subregion`:
- `internal/comparison/resolver/space_type.go` (Task 4)
- `internal/comparison/resolver/university.go` (Task 4)
- `internal/universities/handlers.go` (two calls + the `SpacesService` interface)

Example update in `internal/universities/handlers.go`:

```go
// In the SpacesService interface:
ListSpacesByTypePaginated(ctx context.Context, ID, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error)
CountSpacesByType(ctx context.Context, ID, spaceTypeID pgtype.UUID, search, geoCode, region, subregion string) (int64, error)

// In the two handler call sites:
universities, err := h.spaces.ListSpacesByTypePaginated(r.Context(), ID, spaceType.ID, search, country, "", "", sort, limit32, offset)
...
total, err := h.spaces.CountSpacesByType(r.Context(), ID, spaceType.ID, search, country, "", "")
```

- [ ] **Step 3: Run sqlc and tests**

```bash
make sqlc-gen
go test ./internal/spaces/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/spaces.sql internal/spaces/store.go internal/db/gen/
git commit -m "spaces: add region and subregion filters to ListSpacesByTypePaginated"
```

---

## Task 4: Update JCE resolvers for scope-chain filters

**Files:**
- Modify: `internal/comparison/resolver/space_type.go`
- Modify: `internal/comparison/resolver/university.go`
- Modify: `internal/comparison/resolver/city.go`
- Modify: `internal/comparison/resolver/visa.go`
- Modify: `internal/db/queries/cost_of_living.sql`
- Modify: `internal/costofliving/store.go`
- Modify: `internal/costofliving/service.go`
- Test: `internal/comparison/resolver/resolver_test.go` or new `resolver_scope_test.go`

**Interfaces:**
- Consumes: `filters` map with `country_id`, `state_id`, `city_id`, `university_id`, `region`, `subregion`, `parent_space_id`.
- Produces: `ResolvedItem` lists filtered by deepest available parent.

- [ ] **Step 1: Add shared filter helpers**

In `internal/comparison/resolver/space_type.go` (or `resolver.go` if used elsewhere):

```go
func parseUUIDFilter(filters map[string]any, key string) pgtype.UUID {
    if v, ok := filters[key].(string); ok && v != "" {
        if id, err := ParseUUID(v); err == nil {
            return id
        }
    }
    return pgtype.UUID{}
}

func parseStringFilter(filters map[string]any, key string) string {
    if v, ok := filters[key].(string); ok {
        return v
    }
    return ""
}

func parseBoolFilter(filters map[string]any, key string) bool {
    if v, ok := filters[key].(bool); ok {
        return v
    }
    return false
}
```

- [ ] **Step 2: Update `spaceTypeResolver.Search`**

In `internal/comparison/resolver/space_type.go`:

```go
func (r *spaceTypeResolver) Search(ctx context.Context, query string, filters map[string]any, ID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
    if limit <= 0 {
        limit = defaultSearchLimit
    }
    if limit > MaxSearchLimit {
        limit = MaxSearchLimit
    }
    if offset < 0 {
        offset = 0
    }

    typeID, err := r.typeID(ctx, ID)
    if err != nil {
        return nil, err
    }
    if !typeID.Valid {
        return nil, nil
    }

    search := strings.TrimSpace(query)
    var items any
    var listErr error

    switch r.entityType {
    case "country":
        region := parseStringFilter(filters, "region")
        subregion := parseStringFilter(filters, "subregion")
        rows, err := r.svc.ListSpacesByTypePaginated(ctx, ID, typeID, search, "", region, subregion, "name", limit, offset)
        if err != nil {
            return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
        }
        out := make([]ResolvedItem, 0, len(rows))
        for _, it := range rows {
            if base, ok := spaceBaseFromRow(it); ok {
                out = append(out, resolveSpaceBase(ctx, ID, r.deps, r.entityType, base))
            }
        }
        return out, nil
    case "state":
        countryID := parseUUIDFilter(filters, "country_id")
        if !countryID.Valid {
            rows, err := r.svc.ListSpacesByTypePaginated(ctx, ID, typeID, search, "", "", "", "name", limit, offset)
            if err != nil {
                return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
            }
            items = rows
        } else {
            rows, err := r.svc.ListSpacesByAncestorAndType(ctx, ID, typeID, countryID, search, "name", limit, offset)
            if err != nil {
                return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
            }
            items = rows
        }
    case "city":
        stateID := parseUUIDFilter(filters, "state_id")
        if !stateID.Valid {
            rows, err := r.svc.ListSpacesByTypePaginated(ctx, ID, typeID, search, "", "", "", "name", limit, offset)
            if err != nil {
                return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
            }
            items = rows
        } else {
            rows, err := r.svc.ListSpacesByAncestorAndType(ctx, ID, typeID, stateID, search, "name", limit, offset)
            if err != nil {
                return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
            }
            items = rows
        }
    default:
        rows, err := r.svc.ListSpacesByTypePaginated(ctx, ID, typeID, search, "", "", "", "name", limit, offset)
        if err != nil {
            return nil, fmt.Errorf("searching %s: %w", r.entityType, err)
        }
        items = rows
    }

    out := make([]ResolvedItem, 0)
    switch rows := items.(type) {
    case []gen.ListSpacesByTypePaginatedRow:
        out = make([]ResolvedItem, 0, len(rows))
        for _, it := range rows {
            if base, ok := spaceBaseFromRow(it); ok {
                out = append(out, resolveSpaceBase(ctx, ID, r.deps, r.entityType, base))
            }
        }
    case []gen.ListSpacesByAncestorAndTypeRow:
        out = make([]ResolvedItem, 0, len(rows))
        for _, it := range rows {
            if base, ok := spaceBaseFromRow(it); ok {
                out = append(out, resolveSpaceBase(ctx, ID, r.deps, r.entityType, base))
            }
        }
    }
    return out, listErr
}
```

Also add a `case gen.ListSpacesByAncestorAndTypeRow` to `spaceBaseFromRow` in `internal/comparison/resolver/score_inputs.go`:

```go
case gen.ListSpacesByAncestorAndTypeRow:
    b = spaceBase{
        ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
        Name: r.Name, Slug: r.Slug, Description: r.Description,
        GeoCode: r.GeoCode, Level: r.Level,
        LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
        MemberCount: r.MemberCount,
    }
```

- [ ] **Step 3: Update `universityResolver.Search`**

In `internal/comparison/resolver/university.go`:

```go
func (r *universityResolver) Search(ctx context.Context, query string, filters map[string]any, ID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
    if limit <= 0 {
        limit = defaultSearchLimit
    }
    if limit > MaxSearchLimit {
        limit = MaxSearchLimit
    }
    if offset < 0 {
        offset = 0
    }

    typeID, err := r.universityTypeID(ctx, ID)
    if err != nil {
        return nil, err
    }
    if !typeID.Valid {
        return nil, nil
    }

    // Try ancestors from the deepest selected geo scope upward. Universities may
    // be parented to a city, a state, or only a country, so fall back if the
    // direct ancestor search returns no results (Section 39).
    var items any
    found := false
    for _, key := range []string{"city_id", "state_id", "country_id"} {
        ancestorID := parseUUIDFilter(filters, key)
        if !ancestorID.Valid {
            continue
        }
        rows, err := r.svc.ListSpacesByAncestorAndType(ctx, ID, typeID, ancestorID, query, "name", limit, offset)
        if err != nil {
            return nil, fmt.Errorf("searching universities: %w", err)
        }
        if len(rows) > 0 {
            items = rows
            found = true
            break
        }
    }

    if !found {
        rows, err := r.svc.ListSpacesByTypePaginated(ctx, ID, typeID, query, "", "", "", "name", limit, offset)
        if err != nil {
            return nil, fmt.Errorf("searching universities: %w", err)
        }
        items = rows
    }

    out := make([]ResolvedItem, 0)
    switch rows := items.(type) {
    case []gen.ListSpacesByTypePaginatedRow:
        out = make([]ResolvedItem, 0, len(rows))
        for _, it := range rows {
            if base, ok := spaceBaseFromRow(it); ok {
                out = append(out, resolveSpaceBase(ctx, ID, r.deps, "university", base))
            }
        }
    case []gen.ListSpacesByAncestorAndTypeRow:
        out = make([]ResolvedItem, 0, len(rows))
        for _, it := range rows {
            if base, ok := spaceBaseFromRow(it); ok {
                out = append(out, resolveSpaceBase(ctx, ID, r.deps, "university", base))
            }
        }
    }
    return out, nil
}
```

- [ ] **Step 4: Update `cityResolver.Search`, `ListCOLCities`, `CountCOLCities`, and cost-of-living store**

Update `internal/db/queries/cost_of_living.sql`:

```sql
-- name: ListCOLCities :many
SELECT
    ci.id,
    ci.title,
    ci.description,
    ci.geo_space_id,
    ci.is_baseline,
    ci.last_updated_at,
    ci.position,
    ci.intel_cost_data,
    s.name AS geo_space_name,
    s.slug AS geo_space_slug
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id AND s.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.tenant_id = @tenant_id
  AND ci.type_id = @type_id
  AND ci.deleted_at IS NULL
  AND (@search::text = '' OR ci.title ILIKE '%' || @search || '%')
  AND (@country_code::text = '' OR country.geo_code = @country_code)
  AND (@country_id::uuid IS NULL OR country.id = @country_id)
  AND (@state_id::uuid IS NULL OR state.id = @state_id)
  AND (@city_id::uuid IS NULL OR s.id = @city_id)
ORDER BY ci.is_baseline DESC, ci.title ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCOLCities :one
SELECT COUNT(*) AS count
FROM comparison_items ci
LEFT JOIN spaces s ON s.id = ci.geo_space_id AND s.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_state ON sps_state.space_id = s.id
LEFT JOIN spaces state ON state.id = sps_state.parent_space_id AND state.level = 'state' AND state.deleted_at IS NULL
LEFT JOIN space_parent_spaces sps_country ON sps_country.space_id = state.id
LEFT JOIN spaces country ON country.id = sps_country.parent_space_id AND country.level = 'country' AND country.deleted_at IS NULL
WHERE ci.tenant_id = @tenant_id
  AND ci.type_id = @type_id
  AND ci.deleted_at IS NULL
  AND (@search::text = '' OR ci.title ILIKE '%' || @search || '%')
  AND (@country_code::text = '' OR country.geo_code = @country_code)
  AND (@country_id::uuid IS NULL OR country.id = @country_id)
  AND (@state_id::uuid IS NULL OR state.id = @state_id)
  AND (@city_id::uuid IS NULL OR s.id = @city_id);
```

Update `internal/costofliving/store.go`:

```go
func (s *Store) ListCities(ctx context.Context, ID, typeID pgtype.UUID, search, countryCode string, countryID, stateID, cityID pgtype.UUID, offset, limit int32) ([]gen.ListCOLCitiesRow, error) {
    rows, err := s.queries.ListCOLCities(ctx, gen.ListCOLCitiesParams{
        ID:    ID,
        TypeID:      typeID,
        Search:      search,
        CountryCode: countryCode,
        CountryID:   countryID,
        StateID:     stateID,
        CityID:      cityID,
        PageOffset:  offset,
        PageLimit:   limit,
    })
    if err != nil {
        return nil, fmt.Errorf("costofliving: list cities: %w", err)
    }
    return rows, nil
}

func (s *Store) CountCities(ctx context.Context, ID, typeID pgtype.UUID, search, countryCode string, countryID, stateID, cityID pgtype.UUID) (int64, error) {
    count, err := s.queries.CountCOLCities(ctx, gen.CountCOLCitiesParams{
        ID:    ID,
        TypeID:      typeID,
        Search:      search,
        CountryCode: countryCode,
        CountryID:   countryID,
        StateID:     stateID,
        CityID:      cityID,
    })
    if err != nil {
        return 0, fmt.Errorf("costofliving: count cities: %w", err)
    }
    return count, nil
}
```

Update `internal/costofliving/service.go` `ListCities` to pass `pgtype.UUID{}` for the new ID filters (the public calculator only uses `countryCode`):

```go
rows, err := s.store.ListCities(ctx, ID, typeDef.ID, search, countryCode, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, offset, int32(limit))
```

and `CountCities`:

```go
total, err := s.store.CountCities(ctx, ID, typeDef.ID, search, countryCode, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{})
```

Also update any other `CountCities` call site (e.g. `AdminStats` in `internal/costofliving/service.go`) to pass `pgtype.UUID{}` for the three new ID parameters.

In `internal/comparison/resolver/city.go` `Search`:

```go
var countryCode string
var countryID, stateID, cityID pgtype.UUID
if v, ok := filters["country_code"].(string); ok { countryCode = v }
countryID = parseUUIDFilter(filters, "country_id")
stateID = parseUUIDFilter(filters, "state_id")
cityID = parseUUIDFilter(filters, "city_id")

rows, err := r.store.ListCities(ctx, ID, typeDef.ID, query, countryCode, countryID, stateID, cityID, offset, limit)
```

- [ ] **Step 5: Update `visaRouteResolver.Search`**

In `internal/comparison/resolver/visa.go`:

```go
func (r *visaRouteResolver) Search(ctx context.Context, query string, filters map[string]any, ID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
    if limit <= 0 {
        limit = defaultSearchLimit
    }
    if limit > MaxSearchLimit {
        limit = MaxSearchLimit
    }
    if offset < 0 {
        offset = 0
    }

    var destSpaceID pgtype.UUID
    if v, ok := filters["country_id"].(string); ok && v != "" {
        destSpaceID, _ = ParseUUID(v)
    }

    var category string
    if v, ok := filters["visa_category"].(string); ok {
        category = v
    }

    page := int32(1)
    if limit > 0 {
        page = offset/limit + 1
    }

    routes, _, err := r.svc.AdminList(ctx, ID, destSpaceID, "published", category, query, "", "", boolPtr(true), page, limit)
    if err != nil {
        return nil, fmt.Errorf("searching visa routes: %w", err)
    }

    out := make([]ResolvedItem, 0, len(routes))
    for _, route := range routes {
        out = append(out, routeToResolvedItem(route))
    }
    return out, nil
}
```

- [ ] **Step 6: Add `scope_chain` to `jceTypeConfig`**

In `internal/comparison/jce_service.go`:

```go
type jceTypeConfig struct {
    ItemRefTypes           []string `json:"item_ref_types"`
    DefaultItemRefType     string   `json:"default_item_ref_type"`
    AllowNestedComparisons bool     `json:"allow_nested_comparisons"`
    AllowedNestedTypes     []string `json:"allowed_nested_types"`
    MaxItemCount           int      `json:"max_item_count"`
    DefaultItemCount       int      `json:"default_item_count"`
    ScopeChain             []string `json:"scope_chain"`
}
```

- [ ] **Step 7: Run resolver tests**

```bash
make sqlc-gen
go test ./internal/comparison/resolver/...
```

- [ ] **Step 8: Commit**

```bash
git add internal/comparison/resolver/*.go internal/comparison/jce_service.go internal/db/queries/cost_of_living.sql
git add internal/costofliving/store.go internal/costofliving/service.go
git add internal/db/gen/
git commit -m "comparison: add scope-chain filters to resolvers"
```

---

## Task 5: Create university course catalog and offerings tables

**Files:**
- Modify: `.jeos/sections/39-space-types-university.md`
- Create: `migrations/000590_jce_university_courses.up.sql`
- Create: `migrations/000590_jce_university_courses.down.sql`
- Test: `psql` schema verification

**Interfaces:**
- Produces: `university_course_catalog` and `university_course_offerings` tables.

- [ ] **Step 0: Update JEOS first**

Add the new `university_course_catalog` and `university_course_offerings` schema, relationships, and soft-delete semantics to `.jeos/sections/39-space-types-university.md` before writing the migration. Reference the design spec section 4.4.

- [ ] **Step 1: Write migration**

```sql
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
BEGIN;

CREATE TABLE IF NOT EXISTS university_course_catalog (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title               text NOT NULL,
    slug                text NOT NULL,
    description         text,
    degree_type         text,
    department          text,
    field_of_study      text,
    language            text NOT NULL DEFAULT 'English',
    default_duration_years numeric,
    is_published        boolean NOT NULL DEFAULT true,
    deleted_at          timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_slug ON university_course_catalog(tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_title ON university_course_catalog(tenant_id, title) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS university_course_offerings (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    course_id           uuid NOT NULL REFERENCES university_course_catalog(id) ON DELETE CASCADE,
    university_id       uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    slug                text NOT NULL,
    tuition             numeric,
    currency            text,
    duration_years      numeric,
    study_mode          text,
    intake_months       text[],
    entry_requirements  text,
    accreditation       text,
    is_published        boolean NOT NULL DEFAULT true,
    deleted_at          timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, course_id, university_id),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_university_course_offerings_university ON university_course_offerings(tenant_id, university_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_university_course_offerings_course ON university_course_offerings(tenant_id, course_id) WHERE deleted_at IS NULL;

COMMIT;
```

- [ ] **Step 2: Write down migration**

`migrations/000590_jce_university_courses.down.sql`:

```sql
BEGIN;

DROP TABLE IF EXISTS university_course_offerings;
DROP TABLE IF EXISTS university_course_catalog;

COMMIT;
```

- [ ] **Step 3: Run migration**

```bash
make migrate-up
```

- [ ] **Step 4: Verify**

```bash
psql $DATABASE_URL -c "\dt university_course_*"
```

- [ ] **Step 5: Commit**

```bash
git add .jeos/sections/39-space-types-university.md
make migrate-up  # run once up/down/up is clean
make migrate-down-one && make migrate-up  # verify down migration
make migrate-up
git add migrations/000590_jce_university_courses.up.sql migrations/000590_jce_university_courses.down.sql
git commit -m "migrations: create university course catalog and offerings"
```

---

## Task 6: Build `internal/university_courses` store and service

**Files:**
- Create: `internal/university_courses/models.go`
- Create: `internal/university_courses/store.go`
- Create: `internal/university_courses/service.go`
- Create: `internal/db/queries/university_courses.sql`
- Test: `internal/university_courses/service_test.go` or `university_courses_test.go`

**Interfaces:**
- Produces: `CourseCatalog`, `CourseOffering`, `Store`, `Service`, and `SearchOfferings(ctx, ID, universityID, query, limit, offset)`.

- [ ] **Step 1: Write SQL queries**

`internal/db/queries/university_courses.sql`:

```sql
-- name: ListCourseOfferingsByUniversity :many
SELECT o.id, o.tenant_id, o.course_id, o.university_id, o.slug,
       o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
       o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
       c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
       c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
       s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.tenant_id = @tenant_id AND o.deleted_at IS NULL AND o.is_published = true
  AND c.is_published = true
  AND (@university_id::uuid IS NULL OR o.university_id = @university_id)
  AND (@search::text = '' OR c.title ILIKE '%' || @search || '%')
ORDER BY c.title ASC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetCourseOfferingByID :one
SELECT o.id, o.tenant_id, o.course_id, o.university_id, o.slug,
       o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
       o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
       c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
       c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
       s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.id = @id AND o.tenant_id = @tenant_id AND o.deleted_at IS NULL;

-- name: GetCourseOfferingBySlug :one
SELECT o.id, o.tenant_id, o.course_id, o.university_id, o.slug,
       o.tuition, o.currency, o.duration_years, o.study_mode, o.intake_months,
       o.entry_requirements, o.accreditation, o.is_published, o.created_at, o.updated_at,
       c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
       c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
       s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id AND c.deleted_at IS NULL
JOIN spaces s ON s.id = o.university_id AND s.deleted_at IS NULL
WHERE o.tenant_id = @tenant_id
  AND o.deleted_at IS NULL
  AND o.slug = @slug
LIMIT 1;
```

- [ ] **Step 2: Write models**

`internal/university_courses/models.go`:

```go
package university_courses

import (
    "github.com/jackc/pgx/v5/pgtype"
)

type CourseCatalog struct {
    ID                   pgtype.UUID
    ID             pgtype.UUID
    Title                string
    Slug                 string
    Description          *string
    DegreeType           *string
    Department           *string
    FieldOfStudy         *string
    Language             string
    DefaultDurationYears pgtype.Numeric
    IsPublished          bool
    DeletedAt            pgtype.Timestamptz
    CreatedAt            pgtype.Timestamptz
    UpdatedAt            pgtype.Timestamptz
}

type CourseOffering struct {
    ID                  pgtype.UUID
    ID            pgtype.UUID
    CourseID            pgtype.UUID
    UniversityID        pgtype.UUID
    Slug                string
    Tuition             pgtype.Numeric
    Currency            *string
    DurationYears       pgtype.Numeric
    StudyMode           *string
    IntakeMonths        []string
    EntryRequirements   *string
    Accreditation       *string
    IsPublished         bool
    DeletedAt           pgtype.Timestamptz
    CreatedAt           pgtype.Timestamptz
    UpdatedAt           pgtype.Timestamptz

    // Joined fields
    CourseTitle          string
    CourseSlug           string
    CourseDescription    *string
    DegreeType           *string
    Department           *string
    FieldOfStudy         *string
    Language             string
    DefaultDurationYears pgtype.Numeric
    UniversityName       string
    UniversitySlug       string
}
```

- [ ] **Step 3: Write store**

`internal/university_courses/store.go`:

```go
package university_courses

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/db"
    "github.com/jedafe/japa/internal/db/gen"
)

type Store struct {
    read  *gen.Queries
    write *gen.Queries
    db    *db.DB
}

func NewStore(database *db.DB) *Store {
    return &Store{
        read:  gen.New(database.Reader),
        write: gen.New(database.Writer),
        db:    database,
    }
}

func toCourseOffering(row any) (CourseOffering, error) {
    switch r := row.(type) {
    case gen.ListCourseOfferingsByUniversityRow:
        return CourseOffering{
            ID: r.ID, ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
            Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
            StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
            Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
            CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
            DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
            Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
            UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
        }, nil
    case gen.GetCourseOfferingByIDRow:
        return CourseOffering{
            ID: r.ID, ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
            Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
            StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
            Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
            CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
            DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
            Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
            UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
        }, nil
    case gen.GetCourseOfferingBySlugRow:
        return CourseOffering{
            ID: r.ID, ID: r.ID, CourseID: r.CourseID, UniversityID: r.UniversityID, Slug: r.Slug,
            Tuition: r.Tuition, Currency: r.Currency, DurationYears: r.DurationYears,
            StudyMode: r.StudyMode, IntakeMonths: r.IntakeMonths, EntryRequirements: r.EntryRequirements,
            Accreditation: r.Accreditation, IsPublished: r.IsPublished, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
            CourseTitle: r.CourseTitle, CourseSlug: r.CourseSlug, CourseDescription: r.CourseDescription,
            DegreeType: r.DegreeType, Department: r.Department, FieldOfStudy: r.FieldOfStudy,
            Language: r.Language, DefaultDurationYears: r.DefaultDurationYears,
            UniversityName: r.UniversityName, UniversitySlug: r.UniversitySlug,
        }, nil
    default:
        return CourseOffering{}, fmt.Errorf("unknown course offering row type %T", row)
    }
}

func (s *Store) SearchOfferings(ctx context.Context, ID, universityID pgtype.UUID, query string, limit, offset int32) ([]CourseOffering, error) {
    rows, err := s.read.ListCourseOfferingsByUniversity(ctx, gen.ListCourseOfferingsByUniversityParams{
        ID:     ID,
        UniversityID: universityID,
        Search:       query,
        LimitVal:     limit,
        OffsetVal:    offset,
    })
    if err != nil {
        return nil, fmt.Errorf("university_courses: search offerings: %w", err)
    }
    out := make([]CourseOffering, 0, len(rows))
    for _, r := range rows {
        o, err := toCourseOffering(r)
        if err != nil {
            return nil, err
        }
        out = append(out, o)
    }
    return out, nil
}

func (s *Store) GetOfferingByID(ctx context.Context, ID, id pgtype.UUID) (CourseOffering, error) {
    row, err := s.read.GetCourseOfferingByID(ctx, gen.GetCourseOfferingByIDParams{
        ID:       id,
        ID: ID,
    })
    if err != nil {
        return CourseOffering{}, fmt.Errorf("university_courses: get offering by id: %w", err)
    }
    return toCourseOffering(row)
}

func (s *Store) GetOfferingBySlug(ctx context.Context, ID pgtype.UUID, slug string) (CourseOffering, error) {
    row, err := s.read.GetCourseOfferingBySlug(ctx, gen.GetCourseOfferingBySlugParams{
        Slug:     slug,
        ID: ID,
    })
    if err != nil {
        return CourseOffering{}, fmt.Errorf("university_courses: get offering by slug: %w", err)
    }
    return toCourseOffering(row)
}
```

- [ ] **Step 4: Write service**

`internal/university_courses/service.go`:

```go
package university_courses

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
    store *Store
}

func NewService(store *Store) *Service { return &Service{store: store} }

func (s *Service) SearchOfferings(ctx context.Context, ID, universityID pgtype.UUID, query string, limit, offset int32) ([]CourseOffering, error) {
    return s.store.SearchOfferings(ctx, ID, universityID, query, limit, offset)
}

func (s *Service) GetOfferingByID(ctx context.Context, ID, id pgtype.UUID) (CourseOffering, error) {
    offering, err := s.store.GetOfferingByID(ctx, ID, id)
    if err != nil {
        return CourseOffering{}, fmt.Errorf("fetching course offering: %w", err)
    }
    return offering, nil
}

func (s *Service) GetOfferingBySlug(ctx context.Context, ID pgtype.UUID, slug string) (CourseOffering, error) {
    offering, err := s.store.GetOfferingBySlug(ctx, ID, slug)
    if err != nil {
        return CourseOffering{}, fmt.Errorf("fetching course offering: %w", err)
    }
    return offering, nil
}
```

- [ ] **Step 5: Run sqlc and tests**

```bash
make sqlc-gen
go test ./internal/university_courses/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/university_courses/ internal/db/queries/university_courses.sql
git add internal/db/gen/
git commit -m "university_courses: add store, service, and queries"
```

---

## Task 7: Repoint `course` resolver to university-course offerings

**Files:**
- Modify: `internal/comparison/resolver/course.go`
- Modify: `internal/server/server.go`
- Test: `go test ./internal/comparison/resolver/...`

**Interfaces:**
- Consumes: `university_courses.Service`.
- Produces: `course` resolver that returns offerings with title `{course.title} — {university.name}`.

- [ ] **Step 1: Rewrite resolver**

`internal/comparison/resolver/course.go`:

```go
package resolver

import (
    "context"
    "errors"
    "fmt"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/university_courses"
)

type courseResolver struct {
    svc *university_courses.Service
}

func NewCourseResolver(svc *university_courses.Service) Resolver {
    return &courseResolver{svc: svc}
}

func (r *courseResolver) Resolve(ctx context.Context, ref map[string]any, ID pgtype.UUID) (ResolvedItem, bool, error) {
    idStr, _ := ref["id"].(string)
    slug, _ := ref["slug"].(string)

    if idStr == "" && slug == "" {
        return ResolvedItem{}, false, errors.New("course item_ref missing id or slug")
    }

    var offering university_courses.CourseOffering
    var err error
    if idStr != "" {
        id, perr := ParseUUID(idStr)
        if perr != nil {
            return ResolvedItem{}, false, perr
        }
        offering, err = r.svc.GetOfferingByID(ctx, ID, id)
    } else {
        offering, err = r.svc.GetOfferingBySlug(ctx, ID, slug)
    }

    if err != nil {
        if IsNotFound(err) {
            return ResolvedItem{}, false, nil
        }
        return ResolvedItem{}, false, fmt.Errorf("fetching course offering: %w", err)
    }
    return offeringToResolvedItem(offering), true, nil
}

func (r *courseResolver) Search(ctx context.Context, query string, filters map[string]any, ID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
    if limit <= 0 {
        limit = defaultSearchLimit
    }
    if limit > MaxSearchLimit {
        limit = MaxSearchLimit
    }
    if offset < 0 {
        offset = 0
    }

    universityID := parseUUIDFilter(filters, "university_id")
    if !universityID.Valid {
        return nil, errors.New("university_id filter is required for course search")
    }
    offerings, err := r.svc.SearchOfferings(ctx, ID, universityID, query, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("searching course offerings: %w", err)
    }
    out := make([]ResolvedItem, 0, len(offerings))
    for _, o := range offerings {
        out = append(out, offeringToResolvedItem(o))
    }
    return out, nil
}

func pgNumericToFloat64(n pgtype.Numeric) float64 {
    if !n.Valid {
        return 0
    }
    f, err := n.Float64Value()
    if err != nil {
        return 0
    }
    return f.Float64
}

func offeringToResolvedItem(o university_courses.CourseOffering) ResolvedItem {
    title := o.CourseTitle
    if o.UniversityName != "" {
        title = fmt.Sprintf("%s — %s", o.CourseTitle, o.UniversityName)
    }

    cost := pgNumericToFloat64(o.Tuition)
    var durationWeeks any
    if o.DurationYears.Valid {
        years := pgNumericToFloat64(o.DurationYears)
        if years > 0 {
            durationWeeks = int(years * 52)
        }
    } else if o.DefaultDurationYears.Valid {
        years := pgNumericToFloat64(o.DefaultDurationYears)
        if years > 0 {
            durationWeeks = int(years * 52)
        }
    }

    scoreInputs := map[string]any{
        "cost":           cost,
        "duration_weeks": durationWeeks,
        "language":       []string{o.Language},
        "field":          deref(o.FieldOfStudy),
        "degree_type":    deref(o.DegreeType),
        "university_id":  o.UniversityID.String(),
    }

    return ResolvedItem{
        Type:        "course",
        ID:          o.ID.String(),
        Title:       title,
        Description: Excerpt(deref(o.CourseDescription), 160),
        Metadata: map[string]any{
            "slug":            o.Slug,
            "course_slug":     o.CourseSlug,
            "university_id":   o.UniversityID.String(),
            "university_name": o.UniversityName,
            "university_slug": o.UniversitySlug,
            "degree_type":     deref(o.DegreeType),
            "score_inputs":    scoreInputs,
        },
    }
}
```

- [ ] **Step 2: Wire resolver in server.go**

In `internal/server/server.go`, after the spaces store is wired:

```go
universityCourseStore := university_courses.NewStore(database)
universityCourseService := university_courses.NewService(universityCourseStore)
```

Then in the `jceResolver` registry, replace:

```go
jceResolver.Register("course", resolver.NewCourseResolver(learningStore))
```

with:

```go
jceResolver.Register("course", resolver.NewCourseResolver(universityCourseService))
```

- [ ] **Step 3: Update tests**

```bash
go test ./internal/comparison/resolver/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/comparison/resolver/course.go internal/server/server.go internal/university_courses/
git commit -m "comparison: repoint course resolver to university course offerings"
```

---

## Task 8: Build `ScopeChain` frontend component

**Files:**
- Create: `apps/main/src/lib/components/compare/ScopeChain.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`
- Test: manual component render in `apps/main`

**Interfaces:**
- Consumes: `chain: string[]`, `selections: Record<string,string>`, API helpers.
- Produces: `onChange(key, value)` events.

- [ ] **Step 1: Add translation keys**

`apps/main/src/lib/i18n/en.json`:

```json
{
  "jce.compare.scope.title": "Narrow by location",
  "jce.compare.scope.select_region": "Select region",
  "jce.compare.scope.select_subregion": "Select subregion",
  "jce.compare.scope.select_country": "Select country",
  "jce.compare.scope.select_state": "Select state/region",
  "jce.compare.scope.select_city": "Select city",
  "jce.compare.scope.select_university": "Select university"
}
```

- [ ] **Step 2: Create component**

`apps/main/src/lib/components/compare/ScopeChain.svelte`:

```svelte
<script lang="ts">
  import { api } from '$lib/api/client';
  import { searchEntities } from '$lib/api/jce';
  import { t } from '$lib/i18n';
  import type { ResolvedItem } from '$lib/types/jce';

  type ScopeOption = {
    id: string;
    title: string;
    level?: string;
    geo_code?: string;
  };

  let { chain, selections = $bindable({}), onChange } = $props<{
    chain: string[];
    selections: Record<string, string>;
    onChange: (key: string, value: string) => void;
  }>();

  let options = $state<Record<string, ScopeOption[]>>({});

  function toOption(raw: ResolvedItem): ScopeOption {
    return {
      id: raw.id,
      title: raw.title,
      level: (raw.metadata?.level as string) ?? undefined,
      geo_code: (raw.metadata?.geo_code as string) ?? undefined
    };
  }

  function labelFor(step: string): string {
    return t(`jce.compare.scope.select_${step}`);
  }

  function selectedOption(step: string): ScopeOption | undefined {
    const id = selections[step];
    if (!id) return undefined;
    return options[step]?.find((o) => o.id === id);
  }

  function filtersFor(step: string): Record<string, string> {
    const filters: Record<string, string> = {};
    switch (step) {
      case 'country':
        if (selections.region) filters.region = selections.region;
        if (selections.subregion) filters.subregion = selections.subregion;
        break;
      case 'state':
        if (selections.country) filters.country_id = selections.country;
        break;
      case 'city':
        if (selections.state) filters.state_id = selections.state;
        break;
      case 'university':
        if (selections.city) filters.city_id = selections.city;
        else if (selections.state) filters.state_id = selections.state;
        else if (selections.country) filters.country_id = selections.country;
        break;
    }
    return filters;
  }

  async function loadOptions(step: string) {
    const index = chain.indexOf(step);
    if (index < 0) return;
    const parentKey = chain[index - 1];

    if (step === 'region') {
      const res = await api.get<string[]>('/spaces/regions');
      options[step] = (res.data ?? []).map((r) => ({ id: r, title: r }));
      return;
    }

    if (step === 'subregion') {
      const region = selections.region;
      if (!region) { options[step] = []; return; }
      const res = await api.get<string[]>(`/spaces/subregions?region=${encodeURIComponent(region)}`);
      options[step] = (res.data ?? []).map((r) => ({ id: r, title: r }));
      return;
    }

    if (parentKey && !selections[parentKey]) {
      options[step] = [];
      return;
    }

    const filters = filtersFor(step);
    const items = await searchEntities(step, '', filters, { pageSize: 1000 });
    options[step] = items.map(toOption);
  }

  $effect(() => {
    for (let i = 0; i < chain.length; i++) {
      const step = chain[i];
      const parentKey = chain[i - 1];
      if (i === 0) {
        loadOptions(step);
        continue;
      }
      if (!selections[parentKey]) {
        options[step] = [];
        continue;
      }
      loadOptions(step);
    }
  });
</script>

<div class="flex flex-wrap gap-3">
  {#each chain as step, index (step)}
    {@const parentKey = chain[index - 1]}
    {@const disabled = index > 0 && !selections[parentKey]}
    <select
      class="rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm"
      style="border-color: var(--color-border); color: var(--color-text)"
      {disabled}
      value={selections[step] ?? ''}
      onchange={(e) => onChange(step, e.currentTarget.value)}
    >
      <option value="">{labelFor(step)}</option>
      {#each options[step] ?? [] as opt (opt.id)}
        <option value={opt.id}>{opt.title}</option>
      {/each}
    </select>
  {/each}
</div>
```

- [ ] **Step 3: Verify build**

```bash
cd apps/main && bun run check
```

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/compare/ScopeChain.svelte apps/main/src/lib/i18n/en.json
git commit -m "jce: add ScopeChain component"
```

---

## Task 9: Integrate `ScopeChain` into `/compare`

**Files:**
- Modify: `apps/main/src/lib/types/jce.ts`
- Modify: `apps/main/src/routes/compare/+page.server.ts`
- Modify: `apps/main/src/routes/compare/+page.svelte`
- Create: `tests/e2e/jce-scope-chain.spec.ts`
- Test: `bunx playwright test tests/e2e/jce-scope-chain.spec.ts`

**Interfaces:**
- Consumes: `scope_chain` from type config, `ScopeChain` component.
- Produces: `compare?type=cost_of_living&country_id=...&state_id=...&city_id=...` URL with filtered item list.

- [ ] **Step 1: Update `ComparisonTypeConfig` interface**

```ts
export interface ComparisonTypeConfig {
  max_item_count?: number;
  default_item_count?: number;
  item_ref_types?: string[];
  default_item_ref_type?: string;
  allow_nested_comparisons?: boolean;
  allowed_nested_types?: string[];
  scope_chain?: string[];
  [k: string]: unknown;
}
```

- [ ] **Step 2: Read scope params in `+page.server.ts`**

After reading `nestedType` (line 98), add:

```ts
const scopeParams: Record<string, string> = {};
for (const key of ['country_id', 'state_id', 'city_id', 'university_id', 'region', 'subregion']) {
  const v = searchParams.get(key);
  if (v) scopeParams[key] = v;
}
```

Pass them to `/jce/entities/${source}`:

```ts
for (const [key, value] of Object.entries(scopeParams)) {
  params.set(key, value);
}
```

Also return `scopeParams` and `scopeChain` to the page:

```ts
return {
  // ... existing
  scopeChain: (selectedType?.config?.scope_chain as string[]) ?? [],
  scopeParams,
};
```

- [ ] **Step 3: Render and wire `ScopeChain` in `+page.svelte`**

Add the import near the top:

```ts
import ScopeChain from '$lib/components/compare/ScopeChain.svelte';
```

Add props from `data` and the step/filter key helpers:

```ts
let scopeChain = $derived(data.scopeChain ?? []);

function stepToFilterKey(step: string): string {
  if (step === 'country' || step === 'state' || step === 'city' || step === 'university') {
    return `${step}_id`;
  }
  return step;
}

let scopeSelections = $state<Record<string, string>>({});

$effect(() => {
  const next: Record<string, string> = {};
  for (const step of data.scopeChain ?? []) {
    const key = stepToFilterKey(step);
    if (data.scopeParams?.[key]) next[step] = data.scopeParams[key];
  }
  scopeSelections = next;
});
```

Insert the `ScopeChain` component before the controls (around line 813):

```svelte
{#if scopeChain.length > 0}
  <section class="space-y-3">
    <h2 class="text-sm font-semibold" style="color: var(--color-text)">{t('jce.compare.scope.title')}</h2>
    <ScopeChain chain={scopeChain} selections={scopeSelections} onChange={handleScopeChange} />
  </section>
{/if}
```

Add `handleScopeChange`:

```ts
function handleScopeChange(step: string, value: string) {
  const url = new URL(window.location.href);
  const idx = scopeChain.indexOf(step);
  for (let i = idx; i < scopeChain.length; i++) {
    url.searchParams.delete(stepToFilterKey(scopeChain[i]));
  }
  if (value) {
    url.searchParams.set(stepToFilterKey(step), value);
  }
  url.searchParams.delete('page');
  goto(url, { replaceState: true, keepFocus: true });
}
```

Also clear scope params when changing comparison type or source. Update `selectType` and `selectSource` to delete each `stepToFilterKey(step)` for `data.scopeChain` (or `nextType.config?.scope_chain`). For `selectType`:

```ts
const nextScopeChain = (nextType.config?.scope_chain as string[] | undefined) ?? [];
for (const step of nextScopeChain) {
  url.searchParams.delete(stepToFilterKey(step));
}
```

For `selectSource`:

```ts
for (const step of data.scopeChain ?? []) {
  url.searchParams.delete(stepToFilterKey(step));
}
```

- [ ] **Step 4: Create E2E spec**

`tests/e2e/jce-scope-chain.spec.ts`:

```ts
import { test, expect } from '@playwright/test';

test.describe('JCE scope chain picker', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/compare?type=countries');
  });

  test('countries can be filtered by region and subregion', async ({ page }) => {
    const regionSelect = page.locator('select').first();
    await regionSelect.selectOption('Africa');
    await expect(page).toHaveURL(/region=Africa/);

    const subregionSelect = page.locator('select').nth(1);
    await subregionSelect.selectOption('Western Africa');
    await expect(page).toHaveURL(/subregion=Western%20Africa/);

    await expect(page.locator('text=Nigeria')).toBeVisible();
  });

  test('universities can be filtered by country, state, city', async ({ page }) => {
    await page.goto('/compare?type=universities');
    // Select Nigeria → Lagos State
    await page.locator('select').first().selectOption('Nigeria');
    await page.locator('select').nth(1).selectOption('Lagos');
    await expect(page.locator('text=University of Lagos')).toBeVisible();
  });

  test('cost of living can be filtered to a city', async ({ page }) => {
    await page.goto('/compare?type=cost_of_living');
    await page.locator('select').first().selectOption('Nigeria');
    await page.locator('select').nth(1).selectOption('Lagos');
    await page.locator('select').nth(2).selectOption('Lagos');
    await expect(page).toHaveURL(/city_id=/);
  });

  test('courses can be filtered by university', async ({ page }) => {
    await page.goto('/compare?type=courses');
    await page.locator('select').first().selectOption('University of Lagos');
    await expect(page).toHaveURL(/university_id=/);
  });
});
```

- [ ] **Step 5: Run Svelte checks and e2e smoke**

```bash
cd apps/main && bun run check
cd ../.. && bunx playwright test tests/e2e/jce-nested.spec.ts
```

- [ ] **Step 6: Commit**

```bash
git add apps/main/src/lib/types/jce.ts apps/main/src/routes/compare/+page.server.ts apps/main/src/routes/compare/+page.svelte tests/e2e/jce-scope-chain.spec.ts
git commit -m "jce: integrate ScopeChain into compare page"
```

---

## Task 10: Seed demo data for Lagos, Abuja, Accra, Nairobi

**Files:**
- Create: `migrations/000591_jce_seed_demo_cities.up.sql`
- Create: `migrations/000591_jce_seed_demo_cities.down.sql`
- Test: UI smoke test on `/compare?type=cost_of_living`

**Interfaces:**
- Produces: `comparison_items` of type `cost_of_living` for Abuja, Accra, Nairobi, linked to their city spaces, plus the 9 cost attribute values.

- [ ] **Step 1: Write seed migration**

`migrations/000591_jce_seed_demo_cities.up.sql`:

```sql
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
BEGIN;

-- Create demo city spaces for Lagos, Abuja, Accra, Nairobi and link them to
-- their state/country via space_parent_spaces. Then repoint the existing
-- baseline Lagos item to the new Lagos city space and create cost_of_living
-- comparison_items + 9 cost values for the other three cities.
-- Values are placeholders; update via admin/ingestion later.

WITH city_refs (slug, name, state_slug, country_slug, geo_code) AS (
  VALUES
    ('lagos-city', 'Lagos', 'ng-lagos', 'nigeria', 'LOS'),
    ('abuja', 'Abuja', 'ng-abuja-federal-capital-territory', 'nigeria', 'ABV'),
    ('accra', 'Accra', 'gh-greater-accra', 'ghana', 'ACC'),
    ('nairobi', 'Nairobi', 'ke-nairobi-city', 'kenya', 'NBO')
),
city_spaces AS (
  INSERT INTO spaces (tenant_id, space_type_id, parent_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
  SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE slug = 'city' AND tenant_id = '00000000-0000-0000-0000-000000000001'),
    state.id,
    c.name,
    c.slug,
    c.geo_code,
    'city',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
  FROM city_refs c
  JOIN spaces state ON state.tenant_id = '00000000-0000-0000-0000-000000000001' AND state.slug = c.state_slug AND state.level = 'state'
  WHERE NOT EXISTS (
    SELECT 1 FROM spaces s
    WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001' AND s.slug = c.slug
  )
  RETURNING id, slug
)
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT cs.id, state.id
FROM city_spaces cs
JOIN city_refs c ON c.slug = cs.slug
JOIN spaces state ON state.tenant_id = '00000000-0000-0000-0000-000000000001' AND state.slug = c.state_slug AND state.level = 'state'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ensure each city also has a direct country parent link.
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, country.id
FROM spaces s
JOIN city_refs c ON s.slug = c.slug AND s.tenant_id = '00000000-0000-0000-0000-000000000001'
JOIN spaces country ON country.tenant_id = '00000000-0000-0000-0000-000000000001' AND country.slug = c.country_slug AND country.level = 'country'
WHERE s.level = 'city'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Repoint the existing baseline Lagos item to the new Lagos city space.
UPDATE comparison_items
SET geo_space_id = (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos-city' AND level = 'city')
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND title = 'Lagos'
  AND type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001');

-- The baseline content that owns the 9 cost dimensions.
WITH baseline_content AS (
  SELECT id FROM contents
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
    AND slug = 'cost-of-living-baseline'
  LIMIT 1
),
seeded_cities (city_slug, name, state_slug, country_slug) AS (
  VALUES
    ('abuja', 'Abuja', 'ng-abuja-federal-capital-territory', 'nigeria'),
    ('accra', 'Accra', 'gh-greater-accra', 'ghana'),
    ('nairobi', 'Nairobi', 'ke-nairobi-city', 'kenya')
),
new_items AS (
  INSERT INTO comparison_items (tenant_id, content_id, type_id, title, description, geo_space_id, position, is_baseline, last_updated_at)
  SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM baseline_content),
    (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001'),
    c.name,
    c.name || ' — cost of living demo data',
    s.id,
    0,
    false,
    now()
  FROM seeded_cities c
  JOIN spaces s ON s.tenant_id = '00000000-0000-0000-0000-000000000001' AND s.slug = c.city_slug AND s.level = 'city'
  WHERE NOT EXISTS (
    SELECT 1 FROM comparison_items ci
    WHERE ci.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND ci.type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001')
      AND ci.geo_space_id = s.id
  )
  RETURNING id
)
INSERT INTO comparison_values (dimension_id, item_id, tenant_id, value, value_rating, value_boolean)
SELECT d.id, i.id, '00000000-0000-0000-0000-000000000001',
  CASE d.slug
    WHEN 'rent_1br_city_centre' THEN '500'
    WHEN 'rent_1br_outside' THEN '350'
    WHEN 'groceries_monthly' THEN '200'
    WHEN 'restaurant_meal_cheap' THEN '8'
    WHEN 'transport_monthly' THEN '50'
    WHEN 'utilities_monthly' THEN '80'
    WHEN 'internet_monthly' THEN '30'
    WHEN 'gym_monthly' THEN '40'
    WHEN 'avg_net_salary' THEN '800'
  END,
  NULL,
  NULL
FROM new_items i
CROSS JOIN comparison_dimensions d
WHERE d.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND d.slug IN ('rent_1br_city_centre', 'rent_1br_outside', 'groceries_monthly', 'restaurant_meal_cheap', 'transport_monthly', 'utilities_monthly', 'internet_monthly', 'gym_monthly', 'avg_net_salary')
ON CONFLICT (dimension_id, item_id) DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write down migration**

`migrations/000591_jce_seed_demo_cities.down.sql`:

```sql
BEGIN;

-- Revert the baseline Lagos item back to the Lagos state space before
-- deleting the Lagos city space we created.
UPDATE comparison_items
SET geo_space_id = (
  SELECT id FROM spaces
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'lagos' AND level = 'state'
)
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND title = 'Lagos'
  AND type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001');

DELETE FROM comparison_values
WHERE item_id IN (
  SELECT id FROM comparison_items
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
    AND type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001')
    AND title IN ('Abuja', 'Accra', 'Nairobi')
);

DELETE FROM comparison_items
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND type_id = (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001')
  AND title IN ('Abuja', 'Accra', 'Nairobi');

DELETE FROM space_parent_spaces
WHERE space_id IN (
  SELECT id FROM spaces
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
    AND level = 'city'
    AND slug IN ('lagos-city', 'abuja', 'accra', 'nairobi')
);

DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'city'
  AND slug IN ('lagos-city', 'abuja', 'accra', 'nairobi');

COMMIT;
```

- [ ] **Step 3: Run migration**

```bash
make migrate-up
```

- [ ] **Step 4: Verify**

```bash
psql $DATABASE_URL -c "SELECT title FROM comparison_items WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND type_id = (SELECT id FROM comparison_type_definitions WHERE slug='cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001');"
```

Expected: Lagos, Abuja, Accra, Nairobi.

- [ ] **Step 5: Commit**

```bash
git add migrations/000591_jce_seed_demo_cities.up.sql migrations/000591_jce_seed_demo_cities.down.sql
git commit -m "migrations: seed cost-of-living items and city spaces for Abuja, Accra, Nairobi"
```

---

## Task 10.5: Seed demo universities, course offerings, and visa routes

**Files:**
- Create: `migrations/000592_jce_seed_universities_courses_visas.up.sql`
- Create: `migrations/000592_jce_seed_universities_courses_visas.down.sql`
- Test: UI smoke on `/compare?type=courses` and `/compare?type=visa-types`

**Interfaces:**
- Produces: `university-of-cape-coast` space, city-university parent links for UNILAG and UCC, demo `university_course_catalog` + `university_course_offerings`, and published `visa_routes` for Nigeria, Ghana, and Kenya.

- [ ] **Step 1: Write seed migration**

`migrations/000592_jce_seed_universities_courses_visas.up.sql`:

```sql
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
--      japaconnect-engineering/sections/90-visa-route-comparison.md
BEGIN;

-- Create the University of Cape Coast (UCC) demo space if it does not exist.
INSERT INTO spaces (tenant_id, space_type_id, parent_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university'),
    (SELECT id FROM spaces WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'gh-greater-accra' AND level = 'state'),
    'University of Cape Coast',
    'university-of-cape-coast',
    'GH',
    'custom',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
WHERE NOT EXISTS (
    SELECT 1 FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university-of-cape-coast'
);

-- Parent UCC to Ghana and the Greater Accra state (idempotent).
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT u.id, c.id
FROM spaces u, spaces c
WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND c.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND u.slug = 'university-of-cape-coast'
  AND c.slug = 'ghana'
  AND c.level = 'country'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT u.id, s.id
FROM spaces u, spaces s
WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND u.slug = 'university-of-cape-coast'
  AND s.slug = 'gh-greater-accra'
  AND s.level = 'state'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Link demo universities to their demo cities so city-scoped search finds them.
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT u.id, c.id
FROM spaces u, spaces c
WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND c.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND u.slug IN ('university-of-lagos', 'university-of-cape-coast')
  AND c.slug IN ('lagos-city', 'accra')
  AND c.level = 'city'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Demo course catalog.
INSERT INTO university_course_catalog (tenant_id, title, slug, description, degree_type, department, field_of_study, language, default_duration_years)
SELECT
    '00000000-0000-0000-0000-000000000001',
    v.title,
    v.slug,
    v.description,
    v.degree_type,
    v.department,
    v.field_of_study,
    'English',
    4
FROM (VALUES
    ('Computer Science', 'computer-science', 'Bachelor of Science in Computer Science.', 'Bachelor', 'School of Computing', 'Computer Science'),
    ('Economics', 'economics', 'Bachelor of Arts in Economics.', 'Bachelor', 'Faculty of Social Sciences', 'Economics'),
    ('Medicine and Surgery', 'medicine-and-surgery', 'MB;BS program.', 'Bachelor', 'College of Medicine', 'Medicine')
) AS v(title, slug, description, degree_type, department, field_of_study)
WHERE NOT EXISTS (
    SELECT 1 FROM university_course_catalog
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = v.slug
);

-- Demo course offerings for UNILAG and UCC.
INSERT INTO university_course_offerings (tenant_id, course_id, university_id, slug, tuition, currency, duration_years, study_mode, intake_months, entry_requirements, accreditation)
SELECT
    '00000000-0000-0000-0000-000000000001',
    c.id,
    u.id,
    lower(replace(u.name, ' ', '-')) || '--' || c.slug,
    v.tuition,
    v.currency,
    v.duration_years,
    'Full-time',
    ARRAY['September'],
    'Senior secondary certificate with required science/social science credits.',
    'Accredited by national universities commission.'
FROM (VALUES
    ('computer-science', 'university-of-lagos', 1500, 'USD', 4),
    ('economics', 'university-of-lagos', 1200, 'USD', 4),
    ('medicine-and-surgery', 'university-of-lagos', 3500, 'USD', 6),
    ('computer-science', 'university-of-cape-coast', 1300, 'USD', 4),
    ('economics', 'university-of-cape-coast', 1100, 'USD', 4)
) AS v(course_slug, university_slug, tuition, currency, duration_years)
JOIN university_course_catalog c ON c.tenant_id = '00000000-0000-0000-0000-000000000001' AND c.slug = v.course_slug
JOIN spaces u ON u.tenant_id = '00000000-0000-0000-0000-000000000001' AND u.slug = v.university_slug
WHERE NOT EXISTS (
    SELECT 1 FROM university_course_offerings o
    WHERE o.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND o.course_id = c.id
      AND o.university_id = u.id
);

-- Demo visa routes for Nigeria, Ghana, and Kenya.
INSERT INTO visa_routes (tenant_id, destination_space_id, visa_category, name, slug, eligibility_summary, requirements_json, typical_fee_usd, path_to_pr, renewal_terms, restrictions, gov_source_url, source_url_last_checked, status, is_active)
SELECT
    '00000000-0000-0000-0000-000000000001',
    s.id,
    v.visa_category,
    v.name,
    v.slug,
    v.eligibility_summary,
    '{}'::jsonb,
    v.typical_fee_usd,
    v.path_to_pr,
    'Renew before expiry; typically same category.',
    'Employment may require additional permits.',
    v.gov_source_url,
    CURRENT_DATE,
    'published',
    true
FROM (VALUES
    ('nigeria', 'Work', 'Nigeria Work Visa', 'nigeria-work-visa', 'For skilled workers with a job offer in Nigeria.', 250.00, 'Continuous residence can lead to permanent residence after qualifying period.', 'https://immigration.gov.ng'),
    ('nigeria', 'Student', 'Nigeria Student Visa', 'nigeria-student-visa', 'For students admitted to a Nigerian institution.', 150.00, 'Student residence does not directly lead to PR; switch to work route after graduation.', 'https://immigration.gov.ng'),
    ('ghana', 'Work', 'Ghana Work Permit', 'ghana-work-permit', 'For expatriates employed by a Ghanaian entity.', 300.00, 'Work permit renewals can lead to residence permit and eventual PR.', 'https://www.mint.gov.gh'),
    ('ghana', 'Student', 'Ghana Student Visa', 'ghana-student-visa', 'For international students enrolled in Ghana.', 100.00, 'Student status does not directly lead to PR.', 'https://www.mint.gov.gh'),
    ('kenya', 'Work', 'Kenya Work Permit', 'kenya-work-permit', 'For foreign nationals with an employment contract in Kenya.', 200.00, 'Work permits can be renewed and may lead to permanent residence.', 'https://www.immigration.go.ke'),
    ('kenya', 'Student', 'Kenya Student Visa', 'kenya-student-visa', 'For students accepted by a Kenyan institution.', 100.00, 'Student visa does not directly lead to PR.', 'https://www.immigration.go.ke')
) AS v(country_slug, visa_category, name, slug, eligibility_summary, typical_fee_usd, path_to_pr, gov_source_url)
JOIN spaces s ON s.tenant_id = '00000000-0000-0000-0000-000000000001' AND s.slug = v.country_slug AND s.level = 'country'
WHERE NOT EXISTS (
    SELECT 1 FROM visa_routes vr
    WHERE vr.tenant_id = '00000000-0000-0000-0000-000000000001'
      AND vr.destination_space_id = s.id
      AND vr.slug = v.slug
);

COMMIT;
```

- [ ] **Step 2: Write down migration**

`migrations/000592_jce_seed_universities_courses_visas.down.sql`:

```sql
BEGIN;

DELETE FROM visa_routes
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('nigeria-work-visa', 'nigeria-student-visa', 'ghana-work-permit', 'ghana-student-visa', 'kenya-work-permit', 'kenya-student-visa');

DELETE FROM university_course_offerings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND (slug LIKE 'university-of-lagos--%' OR slug LIKE 'university-of-cape-coast--%');

DELETE FROM university_course_catalog
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('computer-science', 'economics', 'medicine-and-surgery');

DELETE FROM space_parent_spaces
WHERE space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('university-of-lagos', 'university-of-cape-coast')
)
  AND parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('lagos-city', 'accra')
      AND level = 'city'
);

DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'university-of-cape-coast';

COMMIT;
```

- [ ] **Step 3: Run migration**

```bash
make migrate-up
```

- [ ] **Step 4: Verify**

```bash
psql $DATABASE_URL -c "SELECT title, university_name FROM (SELECT c.title, s.name AS university_name FROM university_course_offerings o JOIN university_course_catalog c ON c.id = o.course_id JOIN spaces s ON s.id = o.university_id WHERE o.tenant_id = '00000000-0000-0000-0000-000000000001') q ORDER BY university_name, title;"
```

Expected: at least five rows, including `Computer Science — University of Lagos` and `Computer Science — University of Cape Coast`.

- [ ] **Step 5: Commit**

```bash
git add migrations/000592_jce_seed_universities_courses_visas.up.sql migrations/000592_jce_seed_universities_courses_visas.down.sql
git commit -m "migrations: seed demo universities, course offerings, and visa routes"
```

---

## Task 11: Final integration, build, and tests

**Files:**
- All above
- Test: `make test`, `bunx playwright test tests/e2e/jce-scope-chain.spec.ts`

- [ ] **Step 1: Regenerate sqlc and build backend**

```bash
make sqlc-gen
go build ./...
```

- [ ] **Step 2: Run backend tests**

```bash
go test ./internal/comparison/... ./internal/spaces/... ./internal/university_courses/...
```

- [ ] **Step 3: Run frontend checks**

```bash
cd apps/main && bun run check && bun run build
```

- [ ] **Step 4: Run e2e tests**

```bash
bunx playwright test tests/e2e/jce-nested.spec.ts tests/e2e/jce-scope-chain.spec.ts
```

- [ ] **Step 5: Manual flow verification**

1. `/compare?type=cost_of_living`
2. Select Nigeria → Lagos State → Lagos.
3. Change filter to Ghana → Greater Accra → Accra.
4. Click Compare.
5. Verify quick comparison has two `col_city` items.

- [ ] **Step 6: Stage and review (do not commit for this plan-update task)**

```bash
git add -A
git diff --cached
```

> Per the task request, no commit is made as part of this plan-update pass. The implementation commits are left for the next execution pass.

---

## Self-Review

- [ ] **Spec coverage:** Every section of `2026-09-01-jce-geo-scoped-comparison-picker-design.md` maps to a task above.
  - `scope_chain` config → Task 1
  - spaces endpoint changes → Tasks 2, 3
  - resolver filters → Task 4
  - university course data model → Tasks 5, 6
  - course resolver → Task 7
  - frontend ScopeChain → Tasks 8, 9
  - data seeding → Tasks 10, 10.5
  - integration → Task 11
- [ ] **Placeholder scan:** No `TBD`, `TODO`, or vague steps. All commands and file paths are concrete.
- [ ] **Type consistency:** `ScopeChain` receives `chain: string[]` and `selections: Record<string, string>`. Server returns `scopeChain` and `scopeParams`. Resolvers use consistent filter keys.
- [ ] **AGENTS compliance:** No AI attribution, uses `bun`, sqlc regen, no hardcoded data, soft deletes only.
- [ ] **Geo-hierarchy correctness:** All parent/ancestor traversal uses `space_parent_spaces`, not `spaces.parent_id`.
- [ ] **Model type consistency:** `university_courses` models use `pgtype.Numeric` and `pgtype.Timestamptz`.
- [ ] **Offering slug uniqueness:** `university_course_offerings` has its own `slug` with a `UNIQUE(tenant_id, slug)` constraint so the course resolver can resolve an offering by slug without ambiguity.
- [ ] **University ancestor fallback:** `universityResolver.Search` falls back through `city_id`, `state_id`, `country_id` so universities parented to a state are still discoverable when a city is selected.
- [ ] **Seed migration correctness:** `000591` creates city spaces, uses `comparison_items.type_id`, seeds `comparison_values`, and avoids invalid `ON CONFLICT`. `000592` seeds UCC, demo course offerings, and visa routes.
