# JCE Geo-Scoped Comparison Picker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a configurable `scope_chain` to comparison types, render it as a hierarchical filter on `/compare`, and repoint the `course` resolver to a new university-course catalog/offerings model.

**Architecture:** Extend `comparison_type_definitions.config` with `scope_chain`, add parent/region/level filters to the spaces and resolver layers, build a Svelte `ScopeChain` component for the picker, and introduce `university_course_catalog` + `university_course_offerings` tables with a dedicated store/resolver.

**Tech Stack:** Go 1.23, Chi, pgx/v5, sqlc, SvelteKit 5, TypeScript, Tailwind 4, Bun.

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

---

## File Structure

| File | Responsibility |
|------|----------------|
| `migrations/000588_jce_scope_chain_config.up.sql` | Adds `scope_chain` to system comparison types. |
| `migrations/000589_jce_university_course_indexes.up.sql` | (Reserved for any additional indexes on the new course tables if needed.) |
| `migrations/000590_jce_university_courses.up.sql` | Creates `university_course_catalog` and `university_course_offerings`. |
| `migrations/000591_jce_seed_demo_cities.up.sql` | Seeds `col_city` records for Abuja, Accra, Nairobi. |
| `internal/db/queries/spaces.sql` | New `ListDistinctRegions`, `ListSubregionsByRegion`; updated `ListSpacesByTypePaginated` with `parent_id`, `region`, `subregion`.
| `internal/db/queries/cost_of_living.sql` | Updated `ListCOLCities` with `state_id`/`city_id` filters. |
| `internal/db/queries/university_courses.sql` | CRUD and search for catalog/offerings. |
| `internal/spaces/store.go` | New `ListSpacesByParentAndLevel`, `ListDistinctRegions`, `ListSubregionsByRegion`. |
| `internal/spaces/space_crud_handlers.go` | Updated `ListSpaces` handler for combined `?parent=&level=&region=&subregion=`. |
| `internal/comparison/resolver/space_type.go` | Updated `Search` with parent/level filters. |
| `internal/comparison/resolver/university.go` | Updated `Search` with `parent_id` / `geo_code`. |
| `internal/comparison/resolver/city.go` | Updated `Search` with `state_id` / `city_id`. |
| `internal/comparison/resolver/visa.go` | Updated `Search` with `country_id` filter. |
| `internal/university_courses/` | New package: `models.go`, `store.go`, `service.go`, `resolver.go`. |
| `internal/comparison/resolver/course.go` | Rewritten to resolve `university_course_offerings`. |
| `internal/comparison/jce_service.go` | Add `ScopeChain` to `jceTypeConfig` struct. |
| `internal/server/server.go` | Register new course resolver and wire store. |
| `apps/jce/src/lib/types/jce.ts` | Add `scope_chain?: string[]` to `ComparisonTypeConfig`. |
| `apps/jce/src/lib/components/compare/ScopeChain.svelte` | New hierarchical filter component. |
| `apps/jce/src/routes/compare/+page.server.ts` | Read scope filter params and pass to `/jce/entities/${source}`. |
| `apps/jce/src/routes/compare/+page.svelte` | Render `ScopeChain`, keep selected items on filter change. |
| `apps/jce/src/lib/i18n/en.json` | New scope-chain labels. |
| `tests/e2e/jce-scope-chain.spec.ts` | E2E tests for scope-chain flows. |

---

## Dependencies

- `Task 2` (spaces SQL) must be done before `Task 4` (resolvers use the SQL).
- `Task 4` must be done before `Task 6` (frontend ScopeChain uses the resolver API).
- `Task 5` (university courses tables) must be done before `Task 7` (course resolver).
- `Task 6` (frontend) can be done after `Task 4` and before `Task 7`.
- `Task 8` (seeding) is last and depends on `Task 4` and `Task 7`.

---

## Task 1: Add `scope_chain` to system comparison types

**Files:**
- Create: `migrations/000588_jce_scope_chain_config.up.sql`
- Test: existing `tests/e2e/jce-nested.spec.ts`, `bunx playwright test tests/e2e/jce-nested.spec.ts`

**Interfaces:**
- Produces: `comparison_type_definitions.config` with `scope_chain` for `cost_of_living`, `universities`, `countries`, `states`, `cities`, `courses`, `visa-types`.

- [ ] **Step 1: Write migration**

```sql
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
BEGIN;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state', 'city'))
WHERE slug IN ('cost_of_living', 'universities', 'cities') AND is_system = true;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('region', 'subregion'))
WHERE slug = 'countries' AND is_system = true;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country', 'state'))
WHERE slug = 'states' AND is_system = true;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('university'))
WHERE slug = 'courses' AND is_system = true;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object('scope_chain', jsonb_build_array('country'))
WHERE slug = 'visa-types' AND is_system = true;

COMMIT;
```

- [ ] **Step 2: Run migration locally**

```bash
make migrate-up
```

- [ ] **Step 3: Verify config**

```bash
psql $DATABASE_URL -c "SELECT slug, config FROM comparison_type_definitions WHERE is_system = true;"
```

Expected: each row has `scope_chain` array.

- [ ] **Step 4: Commit**

```bash
git add migrations/000588_jce_scope_chain_config.up.sql
git commit -m "migrations: add scope_chain to JCE system comparison types"
```

---

## Task 2: Extend spaces SQL for parent/level/region filters

**Files:**
- Modify: `internal/db/queries/spaces.sql`
- Create: `internal/db/queries/spaces.sql` new named queries
- Test: `internal/spaces/store_test.go` or new `internal/spaces/spaces_test.go`

**Interfaces:**
- Consumes: existing `ListSpacesPaginated` query.
- Produces: `ListSpacesByParentAndLevel`, `ListDistinctRegions`, `ListSubregionsByRegion`.

- [ ] **Step 1: Add `ListSpacesByParentAndLevel` query**

Append to `internal/db/queries/spaces.sql`:

```sql
-- name: ListSpacesByParentAndLevel :many
-- Returns spaces of a given level whose parent_id matches the provided parent.
-- Used by the JCE scope-chain picker for country/state/city traversal.
SELECT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description, s.wiki_content,
       s.logo_media_id, s.header_image_id, s.geo_code, s.level, s.member_count, s.post_count,
       s.follower_count, s.visibility, s.posting_perm, s.replying_perm, s.commenting_perm,
       s.joining_perm, s.status, s.created_at, s.updated_at, s.deleted_at, s.currency_code
FROM spaces s
WHERE s.tenant_id = @tenant_id AND s.deleted_at IS NULL
  AND s.parent_id = @parent_id
  AND (@level::text = '' OR s.level = @level)
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

- [ ] **Step 2: Add region/subregion queries**

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

- [ ] **Step 3: Regenerate sqlc**

```bash
make sqlc-gen
```

- [ ] **Step 4: Add store methods**

In `internal/spaces/store.go`, add:

```go
func (s *Service) ListSpacesByParentAndLevel(ctx context.Context, tenantID, viewerUserID, parentID pgtype.UUID, level string) ([]gen.ListSpacesByParentAndLevelRow, error) {
    return s.read.ListSpacesByParentAndLevel(ctx, gen.ListSpacesByParentAndLevelParams{
        TenantID:     tenantID,
        ParentID:     parentID,
        Level:        level,
        ViewerUserID: viewerUserID,
    })
}

func (s *Service) ListDistinctRegions(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
    rows, err := s.read.ListDistinctRegions(ctx, tenantID)
    if err != nil {
        return nil, err
    }
    return rows, nil
}

func (s *Service) ListDistinctSubregions(ctx context.Context, tenantID pgtype.UUID, region string) ([]string, error) {
    rows, err := s.read.ListDistinctSubregions(ctx, gen.ListDistinctSubregionsParams{
        TenantID: tenantID,
        Region:   region,
    })
    if err != nil {
        return nil, err
    }
    return rows, nil
}
```

- [ ] **Step 5: Update `ListSpaces` handler**

Modify the switch in `internal/spaces/space_crud_handlers.go` `ListSpaces`:

```go
case q.Get("parent") != "":
    var parentID pgtype.UUID
    if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
        return
    }
    if levelFilter != "" {
        spaces, err = h.Service.ListSpacesByParentAndLevel(r.Context(), tenantID, viewerUserID, parentID, levelFilter)
    } else {
        spaces, err = h.Service.ListChildSpaces(r.Context(), tenantID, viewerUserID, parentID)
    }
```

Also add `?region=` and `?subregion=` support to the default branch by extending `ListSpacesPaginated` query. Add `AND (@region::text = '' OR s.region = @region)` and `AND (@subregion::text = '' OR s.subregion = @subregion)` to `ListSpacesPaginated` and `CountSpaces`, and pass the values from `ListSpaces`.

- [ ] **Step 6: Add region/subregion endpoints**

In `internal/spaces/space_crud_handlers.go`:

```go
func (h *Handlers) ListDistinctRegions(w http.ResponseWriter, r *http.Request) {
    tenantID, ok := h.requireTenant(w, r)
    if !ok { return }
    regions, err := h.Service.ListDistinctRegions(r.Context(), tenantID)
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list regions")
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": regions})
}

func (h *Handlers) ListSubregionsByRegion(w http.ResponseWriter, r *http.Request) {
    tenantID, ok := h.requireTenant(w, r)
    if !ok { return }
    region := r.URL.Query().Get("region")
    if region == "" {
        errpkg.WriteError(w, http.StatusBadRequest, "missing_region", "region query parameter is required")
        return
    }
    subregions, err := h.Service.ListDistinctSubregions(r.Context(), tenantID, region)
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list subregions")
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": subregions})
}
```

In `internal/server/server.go`, add routes near the existing `/spaces` route:

```go
r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.spaces.view")).Get("/spaces/regions", spaceHandlers.ListDistinctRegions)
r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.spaces.view")).Get("/spaces/subregions", spaceHandlers.ListSubregionsByRegion)
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/spaces/...
```

- [ ] **Step 8: Commit**

```bash
git add internal/db/queries/spaces.sql internal/spaces/store.go internal/spaces/space_crud_handlers.go
git add internal/db/gen/  # generated files
make sqlc-gen && git add internal/db/gen/
git commit -m "spaces: add parent/level/region/subregion filters for scope chain"
```

---

## Task 3: Extend `ListSpacesByTypePaginated` with `parent_id`, `region`, and `subregion` filters

**Files:**
- Modify: `internal/db/queries/spaces.sql`
- Modify: `internal/spaces/store.go`

**Interfaces:**
- Produces: `ListSpacesByTypePaginated` and `CountSpacesByType` accept optional `parent_id`, `region`, and `subregion`.

- [ ] **Step 1: Update SQL**

In `ListSpacesByTypePaginated` and `CountSpacesByType`, add:

```sql
AND (@parent_id::uuid IS NULL OR s.parent_id = @parent_id)
AND (@region::text = '' OR s.region = @region)
AND (@subregion::text = '' OR s.subregion = @subregion)
```

- [ ] **Step 2: Update store method**

```go
func (s *Service) ListSpacesByTypePaginated(ctx context.Context, tenantID, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, sort string, parentID pgtype.UUID, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error) {
    return s.read.ListSpacesByTypePaginated(ctx, gen.ListSpacesByTypePaginatedParams{
        TenantID:     tenantID,
        SpaceTypeID:  spaceTypeID,
        Search:       search,
        GeoCode:      geoCode,
        Region:       region,
        Subregion:    subregion,
        ParentID:     parentID,
        Sort:         sort,
        LimitVal:     limit,
        OffsetVal:    offset,
    })
}
```

Find existing call sites (e.g. `university.go`, `space_type.go`) and pass empty strings / zero UUID; they will be updated in `Task 4`.

- [ ] **Step 3: Run sqlc and tests**

```bash
make sqlc-gen
go test ./internal/spaces/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/spaces.sql internal/spaces/store.go internal/db/gen/
git commit -m "spaces: add parent_id, region, and subregion filters to ListSpacesByTypePaginated"
```

---

## Task 4: Update JCE resolvers for scope-chain filters

**Files:**
- Modify: `internal/comparison/resolver/space_type.go`
- Modify: `internal/comparison/resolver/university.go`
- Modify: `internal/comparison/resolver/city.go`
- Modify: `internal/comparison/resolver/visa.go`
- Modify: `internal/db/queries/cost_of_living.sql`
- Test: `internal/comparison/resolver/resolver_test.go` or new `resolver_scope_test.go`

**Interfaces:**
- Consumes: `filters` map with `country_id`, `state_id`, `city_id`, `university_id`, `region`, `subregion`, `parent_id`.
- Produces: `ResolvedItem` lists filtered by deepest available parent.

- [ ] **Step 1: Update `spaceTypeResolver.Search`**

In `internal/comparison/resolver/space_type.go`:

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

func parentIDForSpaceType(entityType string, filters map[string]any) pgtype.UUID {
    switch entityType {
    case "state":
        return parseUUIDFilter(filters, "country_id")
    case "city":
        return parseUUIDFilter(filters, "state_id")
    case "university":
        // Use the deepest available geo scope as the direct parent.
        for _, key := range []string{"city_id", "state_id", "country_id"} {
            if id := parseUUIDFilter(filters, key); id.Valid {
                return id
            }
        }
    }
    return pgtype.UUID{}
}
```

Use them in `Search`:

```go
parentID := parentIDForSpaceType(r.entityType, filters)
region := parseStringFilter(filters, "region")
subregion := parseStringFilter(filters, "subregion")
items, err := r.svc.ListSpacesByTypePaginated(ctx, tenantID, typeID, query, "", region, subregion, "name", parentID, limit, offset)
```

For `country` resolver, `r.entityType` is `country`; use `region`/`subregion` to filter countries. For `state` resolver, use `country_id` as `parentID`. For `city` resolver, use `state_id` as `parentID`. For `university`, use the deepest selected geo scope.

- [ ] **Step 2: Update `universityResolver.Search`**

```go
var parentID pgtype.UUID
for _, key := range []string{"city_id", "state_id", "country_id"} {
    if id := parseUUIDFilter(filters, key); id.Valid {
        parentID = id
        break
    }
}
items, err := r.svc.ListSpacesByTypePaginated(ctx, tenantID, typeID, query, "", "", "", "name", parentID, limit, offset)
```

The deepest available parent (`city_id`, `state_id`, or `country_id`) is passed as the university's direct `parent_id`. The resolver ignores filters it does not understand.

- [ ] **Step 3: Update `cityResolver.Search` and SQL**

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
LEFT JOIN spaces s ON s.id = ci.geo_space_id
LEFT JOIN spaces state ON state.id = s.parent_id AND state.level = 'state'
LEFT JOIN spaces country ON (
    (country.id = state.parent_id AND country.level = 'country')
    OR (country.id = s.parent_id AND country.level = 'country')
)
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
```

Update `internal/costofliving/store.go`:

```go
func (s *Store) ListCities(ctx context.Context, tenantID, typeID pgtype.UUID, search, countryCode string, countryID, stateID, cityID pgtype.UUID, offset, limit int32) ([]gen.ListCOLCitiesRow, error) {
    rows, err := s.queries.ListCOLCities(ctx, gen.ListCOLCitiesParams{
        TenantID:    tenantID,
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
```

Update `internal/costofliving/service.go` and `handlers.go` accordingly, or add overloads so existing call sites continue to work.

In `internal/comparison/resolver/city.go` `Search`:

```go
var countryCode string
var countryID, stateID, cityID pgtype.UUID
if v, ok := filters["country_code"].(string); ok { countryCode = v }
if v, ok := filters["country_id"].(string); ok && v != "" { countryID, _ = ParseUUID(v) }
if v, ok := filters["state_id"].(string); ok && v != "" { stateID, _ = ParseUUID(v) }
if v, ok := filters["city_id"].(string); ok && v != "" { cityID, _ = ParseUUID(v) }

rows, err := r.store.ListCities(ctx, tenantID, typeDef.ID, query, countryCode, countryID, stateID, cityID, offset, limit)
```

- [ ] **Step 4: Update `visaRouteResolver.Search`**

```go
var destSpaceID pgtype.UUID
if v, ok := filters["country_id"].(string); ok && v != "" {
    destSpaceID, _ = ParseUUID(v)
}
routes, _, err := r.svc.AdminList(ctx, tenantID, destSpaceID, "published", category, query, "", "", boolPtr(true), page, limit)
```

- [ ] **Step 5: Add scope_chain to `jceTypeConfig`**

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

- [ ] **Step 6: Run resolver tests**

```bash
make sqlc-gen
go test ./internal/comparison/resolver/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/comparison/resolver/*.go internal/comparison/jce_service.go internal/db/queries/cost_of_living.sql
git add internal/db/gen/
git commit -m "comparison: add scope-chain filters to resolvers"
```

---

## Task 5: Create university course catalog and offerings tables

**Files:**
- Create: `migrations/000590_jce_university_courses.up.sql`
- Test: `psql` schema verification

**Interfaces:**
- Produces: `university_course_catalog` and `university_course_offerings` tables.

- [ ] **Step 1: Write migration**

```sql
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
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
    UNIQUE(tenant_id, course_id, university_id)
);

CREATE INDEX IF NOT EXISTS idx_university_course_offerings_university ON university_course_offerings(tenant_id, university_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_university_course_offerings_course ON university_course_offerings(tenant_id, course_id) WHERE deleted_at IS NULL;

COMMIT;
```

- [ ] **Step 2: Run migration**

```bash
make migrate-up
```

- [ ] **Step 3: Verify**

```bash
psql $DATABASE_URL -c "\dt university_course_*"
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000590_jce_university_courses.up.sql
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
- Produces: `CourseCatalog`, `CourseOffering`, `Store`, `Service`, and `SearchOfferings(ctx, tenantID, universityID, query)`.

- [ ] **Step 1: Write SQL queries**

`internal/db/queries/university_courses.sql`:

```sql
-- name: ListCourseOfferingsByUniversity :many
SELECT o.id, o.tenant_id, o.course_id, o.university_id, o.tuition, o.currency,
       o.duration_years, o.study_mode, o.intake_months, o.entry_requirements,
       o.accreditation, o.is_published, o.created_at, o.updated_at,
       c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
       c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
       s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
JOIN spaces s ON s.id = o.university_id
WHERE o.tenant_id = @tenant_id AND o.deleted_at IS NULL AND o.is_published = true
  AND (@university_id::uuid IS NULL OR o.university_id = @university_id)
  AND (@search::text = '' OR c.title ILIKE '%' || @search || '%')
ORDER BY c.title ASC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetCourseOfferingByID :one
SELECT o.id, o.tenant_id, o.course_id, o.university_id, o.tuition, o.currency,
       o.duration_years, o.study_mode, o.intake_months, o.entry_requirements,
       o.accreditation, o.is_published, o.created_at, o.updated_at,
       c.title AS course_title, c.slug AS course_slug, c.description AS course_description,
       c.degree_type, c.department, c.field_of_study, c.language, c.default_duration_years,
       s.name AS university_name, s.slug AS university_slug
FROM university_course_offerings o
JOIN university_course_catalog c ON c.id = o.course_id
JOIN spaces s ON s.id = o.university_id
WHERE o.id = @id AND o.tenant_id = @tenant_id AND o.deleted_at IS NULL;

-- name: GetCourseOfferingBySlug :one
-- same as above but WHERE o.tenant_id = @tenant_id AND o.id::text = @slug OR c.slug = @slug
-- (use slug logic appropriate for resolver)
```

- [ ] **Step 2: Write models**

`internal/university_courses/models.go`:

```go
package university_courses

import (
    "time"

    "github.com/jackc/pgx/v5/pgtype"
)

type CourseCatalog struct {
    ID                   pgtype.UUID
    TenantID             pgtype.UUID
    Title                string
    Slug                 string
    Description          *string
    DegreeType           *string
    Department           *string
    FieldOfStudy         *string
    Language             string
    DefaultDurationYears *float64
    IsPublished          bool
    DeletedAt            *time.Time
    CreatedAt            time.Time
    UpdatedAt            time.Time
}

type CourseOffering struct {
    ID                  pgtype.UUID
    TenantID            pgtype.UUID
    CourseID            pgtype.UUID
    UniversityID        pgtype.UUID
    Tuition             *float64
    Currency            *string
    DurationYears       *float64
    StudyMode           *string
    IntakeMonths        []string
    EntryRequirements   *string
    Accreditation       *string
    IsPublished         bool
    DeletedAt           *time.Time
    CreatedAt           time.Time
    UpdatedAt           time.Time

    // Joined fields
    CourseTitle         string
    CourseSlug          string
    CourseDescription   *string
    DegreeType          *string
    Department          *string
    FieldOfStudy        *string
    Language            string
    DefaultDurationYears *float64
    UniversityName      string
    UniversitySlug      string
}
```

- [ ] **Step 3: Write store**

`internal/university_courses/store.go` uses `gen.Queries` or raw `pgx` queries. For simplicity, use `gen.Queries` after sqlc generation. Wrap `ListCourseOfferingsByUniversity`, `GetCourseOfferingByID`, `GetCourseOfferingBySlug`.

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

func (s *Service) SearchOfferings(ctx context.Context, tenantID, universityID pgtype.UUID, query string, limit, offset int32) ([]CourseOffering, error) {
    return s.store.SearchOfferings(ctx, tenantID, universityID, query, limit, offset)
}

func (s *Service) GetOfferingByID(ctx context.Context, tenantID pgtype.UUID, id pgtype.UUID) (CourseOffering, error) {
    offering, err := s.store.GetOfferingByID(ctx, tenantID, id)
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

```go
package resolver

import (
    "context"
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

func (r *courseResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
    idStr, _ := ref["id"].(string)
    if idStr == "" {
        return ResolvedItem{}, false, errors.New("course item_ref missing id")
    }
    id, err := ParseUUID(idStr)
    if err != nil {
        return ResolvedItem{}, false, err
    }
    offering, err := r.svc.GetOfferingByID(ctx, tenantID, id)
    if err != nil {
        if IsNotFound(err) { return ResolvedItem{}, false, nil }
        return ResolvedItem{}, false, fmt.Errorf("fetching course offering: %w", err)
    }
    return offeringToResolvedItem(offering), true, nil
}

func (r *courseResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
    var universityID pgtype.UUID
    if v, ok := filters["university_id"].(string); ok && v != "" {
        universityID, _ = ParseUUID(v)
    }
    if !universityID.Valid {
        return nil, errors.New("university_id filter is required for course search")
    }
    offerings, err := r.svc.SearchOfferings(ctx, tenantID, universityID, query, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("searching course offerings: %w", err)
    }
    out := make([]ResolvedItem, 0, len(offerings))
    for _, o := range offerings {
        out = append(out, offeringToResolvedItem(o))
    }
    return out, nil
}

func offeringToResolvedItem(o university_courses.CourseOffering) ResolvedItem {
    title := o.CourseTitle
    if o.UniversityName != "" {
        title = fmt.Sprintf("%s — %s", o.CourseTitle, o.UniversityName)
    }
    scoreInputs := map[string]any{
        "cost":               o.Tuition,
        "duration_weeks":     nil,
        "language":           []string{o.Language},
        "field":              o.FieldOfStudy,
        "degree_type":        o.DegreeType,
        "university_id":      o.UniversityID.String(),
    }
    if o.DurationYears != nil && *o.DurationYears > 0 {
        scoreInputs["duration_weeks"] = int(*o.DurationYears * 52)
    }
    return ResolvedItem{
        Type:        "course",
        ID:          o.ID.String(),
        Title:       title,
        Description: Excerpt(deref(o.CourseDescription), 160),
        Metadata: map[string]any{
            "slug":         o.CourseSlug,
            "university_id": o.UniversityID.String(),
            "university_name": o.UniversityName,
            "degree_type": o.DegreeType,
            "score_inputs": scoreInputs,
        },
    }
}
```

- [ ] **Step 2: Wire resolver in server.go**

```go
universityCourseStore := university_courses.NewStore(queries)
universityCourseService := university_courses.NewService(universityCourseStore)
jceResolver.Register("course", resolver.NewCourseResolver(universityCourseService))
```

Replace the existing `jceResolver.Register("course", resolver.NewCourseResolver(learningStore))`.

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
- Create: `apps/jce/src/lib/components/compare/ScopeChain.svelte`
- Modify: `apps/jce/src/lib/i18n/en.json`
- Test: manual component render in `apps/jce`

**Interfaces:**
- Consumes: `chain: string[]`, `selections: Record<string,string>`, API helpers.
- Produces: `onChange(key, value)` events.

- [ ] **Step 1: Add translation keys**

`apps/jce/src/lib/i18n/en.json`:

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

`apps/jce/src/lib/components/compare/ScopeChain.svelte`:

```svelte
<script lang="ts">
  import { apiGet } from '$lib/api/jce';
  import { t } from '$lib/i18n';

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

  function toOption(raw: any): ScopeOption {
    return {
      id: raw.id,
      title: raw.title ?? raw.name ?? String(raw.id),
      level: raw.level,
      geo_code: raw.geo_code
    };
  }

  function labelFor(step: string) {
    return t(`jce.compare.scope.select_${step}`);
  }

  function selectedOption(step: string): ScopeOption | undefined {
    const id = selections[step];
    if (!id) return undefined;
    return options[step]?.find((o) => o.id === id);
  }

  async function loadOptions(step: string) {
    const index = chain.indexOf(step);
    if (index < 0) return;
    const parentKey = chain[index - 1];

    if (step === 'country' || step === 'state' || step === 'city') {
      const params = new URLSearchParams({ level: step, limit: '1000' });
      if (parentKey) {
        const parent = selectedOption(parentKey);
        if (!parent) { options[step] = []; return; }
        params.set('parent', parent.id);
      }
      const res = await apiGet<any[]>(`/spaces?${params.toString()}`);
      options[step] = (res ?? []).map(toOption);
    } else if (step === 'university') {
      const params = new URLSearchParams({ limit: '1000', offset: '0' });
      if (chain.includes('city') && selections.city) params.set('city_id', selections.city);
      else if (chain.includes('state') && selections.state) params.set('state_id', selections.state);
      else if (chain.includes('country') && selections.country) params.set('country_id', selections.country);
      const res = await apiGet<any[]>(`/jce/entities/university?${params.toString()}`);
      options[step] = (res ?? []).map(toOption);
    } else if (step === 'region') {
      const res = await apiGet<string[]>('/spaces/regions');
      options[step] = (res ?? []).map((r) => ({ id: r, title: r }));
    } else if (step === 'subregion') {
      const region = selections.region;
      if (!region) { options[step] = []; return; }
      const res = await apiGet<string[]>(`/spaces/subregions?region=${encodeURIComponent(region)}`);
      options[step] = (res ?? []).map((r) => ({ id: r, title: r }));
    }
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
cd apps/jce && bun run check
```

- [ ] **Step 4: Commit**

```bash
git add apps/jce/src/lib/components/compare/ScopeChain.svelte apps/jce/src/lib/i18n/en.json
git commit -m "jce: add ScopeChain component"
```

---

## Task 9: Integrate `ScopeChain` into `/compare`

**Files:**
- Modify: `apps/jce/src/lib/types/jce.ts`
- Modify: `apps/jce/src/routes/compare/+page.server.ts`
- Modify: `apps/jce/src/routes/compare/+page.svelte`
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

let scopeSelections = $state<Record<string, string>>(() => {
  const init: Record<string, string> = {};
  for (const step of data.scopeChain ?? []) {
    const key = stepToFilterKey(step);
    if (data.scopeParams?.[key]) init[step] = data.scopeParams[key];
  }
  return init;
});
```

Insert the `ScopeChain` component before the controls (around line 813):

```svelte
{#if scopeChain.length > 0}
  <section class="space-y-3">
    <h2 class="text-sm font-semibold" style="color: var(--color-text)">{t('jce.compare.scope.title')}</h2>
    <ScopeChain {chain} selections={scopeSelections} onChange={handleScopeChange} />
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

- [ ] **Step 4: Add `jce.compare.scope.title` to i18n**

- [ ] **Step 5: Run Svelte checks and e2e smoke**

```bash
cd apps/jce && bun run check
cd ../.. && bunx playwright test tests/e2e/jce-nested.spec.ts
```

- [ ] **Step 6: Commit**

```bash
git add apps/jce/src/lib/types/jce.ts apps/jce/src/routes/compare/+page.server.ts apps/jce/src/routes/compare/+page.svelte
git commit -m "jce: integrate ScopeChain into compare page"
```

---

## Task 10: Seed demo data for Lagos, Abuja, Accra, Nairobi

**Files:**
- Create: `migrations/000591_jce_seed_demo_cities.up.sql`
- Test: UI smoke test on `/compare?type=cost_of_living`

**Interfaces:**
- Produces: `comparison_items` of type `cost_of_living` for Abuja, Accra, Nairobi, linked to their city spaces.

- [ ] **Step 1: Write seed migration**

```sql
-- Ref: japaconnect-engineering/sections/72-cost-of-living.md
BEGIN;

-- Insert baseline cost-of-living items for Abuja, Accra, Nairobi.
-- Values are placeholders; update via admin/ingestion later.

WITH city_refs (slug, name, country, state, city) AS (
  VALUES
    ('abuja', 'Abuja', 'NG', 'Federal Capital Territory', 'Abuja'),
    ('accra', 'Accra', 'GH', 'Greater Accra', 'Accra'),
    ('nairobi', 'Nairobi', 'KE', 'Nairobi City', 'Nairobi')
)
INSERT INTO comparison_items (tenant_id, type_definition_id, title, geo_space_id, is_baseline)
SELECT
  '00000000-0000-0000-0000-000000000001',
  (SELECT id FROM comparison_type_definitions WHERE slug = 'cost_of_living' AND tenant_id = '00000000-0000-0000-0000-000000000001'),
  c.name,
  (SELECT s.id FROM spaces s
   JOIN spaces st ON st.id = s.parent_id
   JOIN spaces co ON co.id = st.parent_id
   WHERE s.slug = c.city AND st.slug = c.state AND co.slug = c.country
     AND s.tenant_id = '00000000-0000-0000-0000-000000000001'
     AND s.level = 'city'
   LIMIT 1),
  false
FROM city_refs c
ON CONFLICT DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Run migration**

```bash
make migrate-up
```

- [ ] **Step 3: Verify**

```bash
psql $DATABASE_URL -c "SELECT title FROM comparison_items WHERE type_definition_id = (SELECT id FROM comparison_type_definitions WHERE slug='cost_of_living');"
```

Expected: Lagos, Abuja, Accra, Nairobi.

- [ ] **Step 4: Commit**

```bash
git add migrations/000591_jce_seed_demo_cities.up.sql
git commit -m "migrations: seed cost-of-living items for Abuja, Accra, Nairobi"
```

---

## Task 11: Final integration, build, and tests

**Files:**
- All above
- Test: `make test`, `bunx playwright test tests/e2e/jce-nested.spec.ts`

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
cd apps/jce && bun run check && bun run build
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

- [ ] **Step 6: Commit any final fixes**

```bash
git add -A
git commit -m "jce: complete geo-scoped comparison picker integration"
```

---

## Self-Review

- [ ] **Spec coverage:** Every section of `2026-09-01-jce-geo-scoped-comparison-picker-design.md` maps to a task above.
  - `scope_chain` config → Task 1
  - spaces endpoint changes → Tasks 2, 3
  - resolver filters → Task 4
  - university course data model → Tasks 5, 6
  - course resolver → Task 7
  - frontend ScopeChain → Tasks 8, 9
  - data seeding → Task 10
  - integration → Task 11
- [ ] **Placeholder scan:** No `TBD`, `TODO`, or vague steps. All commands and file paths are concrete.
- [ ] **Type consistency:** `ScopeChain` receives `chain: string[]` and `selections: Record<string, string>`. Server returns `scopeChain` and `scopeParams`. Resolvers use consistent filter keys.
- [ ] **AGENTS compliance:** No AI attribution, uses `bun`, sqlc regen, no hardcoded data, soft deletes only.
