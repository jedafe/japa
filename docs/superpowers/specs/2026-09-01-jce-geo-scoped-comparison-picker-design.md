# JCE Geo-Scoped Comparison Picker

**Date:** 2026-09-01  
**Status:** Design  
**Scope:** `apps/main`, `internal/comparison`, `internal/spaces`, `internal/learning`, `internal/visaroutes`, `internal/costofliving`, `migrations`

## 1. Problem

The Japa Comparison Engine (`/compare`) currently supports multi-item selection, but the item picker is flat: a search box plus pagination. It does not let the user navigate a hierarchy (Country → State/Region → City, or Region → Subregion → Country, or University) to find comparable items. This makes it impossible to perform the following flows out of the box:

- Compare cost of living between Lagos (Nigeria) and Accra (Ghana).
- Compare universities in Nigeria vs universities in Ghana.
- Compare countries within a region/subregion.
- Compare courses offered by two different universities.
- Compare visa types for two different destination countries.

Additionally, the `courses` comparison type currently resolves to Japa learning tracks. It should resolve to courses offered by real-world universities (e.g. University of Lagos, University of Cape Coast).

## 2. Goal

Add a configurable `scope_chain` to every comparison type. The `/compare` picker renders the chain as a sequence of filters. Each filter step narrows the next until the user reaches the leaf item type, which is the actual comparable entity.

For comparison types that are naturally global, the chain is empty and the picker behaves as it does today.

## 3. Scope chain config

Add `scope_chain` to `comparison_type_definitions.config` (`jsonb`). It is an ordered array of strings where each string is either:

- a **space level**: `country`, `state`, `city`, `region`, `subregion`
- an **entity type**: `university` (and any future parentable entity)

The leaf item type is the first non-`comparison` entry in the type’s existing `item_ref_types` array (`col_city`, `university`, `course`, `visa_route`, `country`, `state`, `city`, etc.).

Example config:

```json
{
  "item_ref_types": ["col_city", "comparison"],
  "scope_chain": ["country", "state", "city"]
}
```

### 3.1 Default scope chains for system comparison types

| Comparison type | `scope_chain` | Leaf item ref |
|-----------------|---------------|---------------|
| `cost_of_living` | `country`, `state`, `city` | `col_city` |
| `universities` | `country`, `state`, `city` | `university` |
| `countries` | `region`, `subregion` | `country` |
| `states` | `country`, `state` | `state` |
| `cities` | `country`, `state`, `city` | `city` |
| `courses` | `university` | `course` |
| `visa-types` | `country` | `visa_route` |
| `banks`, `mentors` | (empty / global) | `organization`, `mentor` |

## 4. Backend design

### 4.1 Spaces endpoint: region / subregion / parent + level

The existing `GET /api/v1/spaces` endpoint supports `?level=`, `?parent=`, and `?geo=`, but the `parent` branch does not filter by `level` and the default branch does not support `parent`. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/spaces/space_crud_handlers.go" lines="29-90" />

Changes:

- Extend the SQL pagination query `ListSpacesPaginated` to support `parent_id` and `region`/`subregion` filters. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/db/queries/spaces.sql" lines="102-138" />
- Extend the `ListSpaces` handler so `?parent={id}&level={level}` and `?region={region}&subregion={subregion}&level=country` work together.
- Add an endpoint or query that returns distinct `region` values and, for a selected region, distinct `subregion` values, so the frontend can render the first two dropdowns for the `countries` scope chain. The `spaces` table already has `region` and `subregion` columns. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/migrations/000529_country_region.up.sql" lines="5-8" />

### 4.2 JCE entity search: parent filters

`GET /api/v1/jce/entities/{type}` already passes arbitrary query params into the resolver `filters` map. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/comparison/jce_handlers.go" lines="578-586" /> We will pass the selected scope chain values through this map.

Filter keys by step:

- `country_id`, `state_id`, `city_id` (or `parent_space_id` + `level`)
- `university_id`
- `region`, `subregion`

Each resolver interprets the filters it understands and ignores the rest. Resolvers should use the deepest available parent. For example, if `country_id` and `state_id` are provided but `city_id` is empty, the `university` resolver returns universities whose parent is the selected state (or any city under that state).

### 4.3 Resolver changes

#### `country`, `state`, `city` (`spaceTypeResolver`)

Update `Search` to accept `parent_id` and `level` filters. Use `ListSpacesByTypePaginated` and `ListChildSpaces` as needed. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/comparison/resolver/space_type.go" lines="104-135" /> <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/spaces/store.go" lines="486-499" />

#### `col_city` (`cityResolver`)

Extend `ListCOLCities` to traverse the space parent chain. Current query joins `spaces`, `state`, and `country`. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/db/queries/cost_of_living.sql" lines="20-45" /> Add optional `state_id` and `city_id` parameters.

#### `university` (`universityResolver`)

Add `parent_id` / `geo_code` filter to find universities in a selected city/state/country. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/comparison/resolver/university.go" lines="85-116" /> <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/db/queries/spaces.sql" lines="205-236" />

#### `course` (new resolver backed by university-course offerings)

Replace the existing `course` resolver that reads `learning_tracks`. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/comparison/resolver/course.go" lines="55-84" />

New resolver:

- Resolves a `university_course_offering` by ID/slug.
- `Search` accepts `university_id` and `q` (query by course title).
- Returns `ResolvedItem` with title `{course.title} — {university.name}` and score inputs (tuition, duration, language, field, degree).

#### `visa_route` (`visaRouteResolver`)

The `visaroutes.Service.AdminList` already accepts a `destSpaceID` (destination country). <ref_snippet file="/home/jmobile/websites/japa/japaconnect/internal/visaroutes/service.go" line="233" /> Update the resolver `Search` to pass the selected `country_id` as `destSpaceID`.

### 4.4 University course data model

A single course can be offered by multiple universities, so the data model is split into a canonical catalog and per-university offerings.

#### `university_course_catalog`

| Column | Type | Notes |
|--------|------|-------|
| `id` | uuid | PK |
| `tenant_id` | uuid | FK → tenants |
| `title` | text | e.g. "BSc Computer Science" |
| `slug` | text | unique per tenant |
| `description` | text | optional |
| `degree_type` | text | e.g. "BSc", "MSc", "PhD", "Diploma" |
| `department` | text | optional |
| `field_of_study` | text | optional |
| `language` | text | default "English" |
| `default_duration_years` | numeric | optional |
| `is_published` | boolean | default true |
| `deleted_at` | timestamptz | soft delete |
| `created_at` | timestamptz | |
| `updated_at` | timestamptz | |

#### `university_course_offerings`

| Column | Type | Notes |
|--------|------|-------|
| `id` | uuid | PK |
| `tenant_id` | uuid | FK → tenants |
| `course_id` | uuid | FK → university_course_catalog |
| `university_id` | uuid | FK → spaces (level = university) |
| `tuition` | numeric | optional |
| `currency` | text | e.g. "USD" |
| `duration_years` | numeric | can override catalog |
| `study_mode` | text | optional: "full-time", "part-time", "online" |
| `intake_months` | text[] | optional |
| `entry_requirements` | text | optional |
| `accreditation` | text | optional |
| `is_published` | boolean | default true |
| `deleted_at` | timestamptz | soft delete |
| `created_at` | timestamptz | |
| `updated_at` | timestamptz | |

Unique: `(tenant_id, course_id, university_id)`.

#### Service and store

- `internal/learning` or a new `internal/university_courses` package.
- `GetOfferingByID`, `GetOfferingBySlug`, `SearchOfferings` (by `university_id` and/or query).
- `CreateCatalog`, `UpdateCatalog`, `CreateOffering`, `UpdateOffering`, `DeleteOffering`.

## 5. Frontend design

### 5.1 `ScopeChain` component

Add `apps/main/src/lib/components/compare/ScopeChain.svelte`.

Props:
- `chain: string[]` — the `scope_chain` from the comparison type config.
- `selections: Record<string, string>` — current selected IDs/values.
- `onChange: (key, value) => void`.

Behavior:
- For each step in the chain, render a dropdown.
- A dropdown is disabled until all previous steps are selected.
- Selecting a step fetches the options for the next step from the backend.
- The last step is **optional**: the user can stop at any level and the item grid is filtered by whatever selections have been made.
- The leaf item grid always appears below the chain. Selecting a chain value narrows the grid; selecting an item in the grid adds it to the comparison.

### 5.2 `compare/+page.svelte`

- When `selectedType.config.scope_chain` is non-empty, render `ScopeChain` above the existing search box.
- Add URL query params for each selected scope step (`country_id`, `state_id`, `city_id`, `university_id`, `region`, `subregion`).
- Changing a scope filter triggers a server reload to fetch the narrowed item list, but does **not** clear `selectedItems`.
- The existing `max_item_count` selection logic remains unchanged. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/apps/main/src/routes/compare/+page.svelte" lines="151" /> <ref_snippet file="/home/jmobile/websites/japa/japaconnect/apps/main/src/routes/compare/+page.svelte" lines="339-344" />

### 5.3 `compare/+page.server.ts`

- Read scope filter IDs from `searchParams`.
- Pass them as additional query params to `apiGet(ctx, /jce/entities/${source})`. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/apps/main/src/routes/compare/+page.server.ts" lines="100-113" />

## 6. Example flows

### 6.1 Cost of living: Lagos vs Accra

1. User selects `cost_of_living`.
2. `ScopeChain` shows Country → State → City.
3. User picks Nigeria → Lagos State → Lagos. Selects Lagos.
4. User clears/changes filter to Ghana → Greater Accra → Accra. Selects Accra.
5. Clicks **Compare**.
6. Backend creates a quick comparison of type `cost_of_living` with two `col_city` item refs.

### 6.2 Universities: UNILAG vs University of Cape Coast

1. User selects `universities`.
2. `ScopeChain` shows Country → State → City.
3. User picks Nigeria → Lagos State → Lagos. Picker lists universities in Lagos.
4. Selects University of Lagos.
5. User changes filter to Ghana → Central Region → Cape Coast. Picker lists universities in Cape Coast.
6. Selects University of Cape Coast.
7. Clicks **Compare**.

### 6.3 Courses: BSc Computer Science at UNILAG vs same at UCC

1. User selects `courses`.
2. `ScopeChain` shows University.
3. User selects University of Lagos. Picker lists all course offerings for UNILAG.
4. User selects BSc Computer Science.
5. User changes filter to University of Cape Coast. Picker lists UCC offerings.
6. User selects BSc Computer Science.
7. Clicks **Compare**.

### 6.4 Visa types: Nigeria vs Canada

1. User selects `visa-types`.
2. `ScopeChain` shows Country.
3. User picks Nigeria. Picker lists visa routes to Nigeria.
4. User selects relevant visa route.
5. User changes filter to Canada. Picker lists visa routes to Canada.
6. User selects relevant visa route.
7. Clicks **Compare**.

### 6.5 Countries: all countries in Western Africa

1. User selects `countries`.
2. `ScopeChain` shows Region → Subregion.
3. User picks Africa → Western Africa. Picker lists countries in that subregion.
4. User selects 2–5 countries.
5. Clicks **Compare**.

## 7. Data ingestion

The picker can only return items that exist. Initial seeding/ingestion requirements:

- `col_city` records for Lagos, Abuja, Accra, Nairobi, and other target cities, linked to the correct city `geo_space_id`. Only Lagos is seeded today. <ref_snippet file="/home/jmobile/websites/japa/japaconnect/migrations/000364_seed_cost_of_living_baseline.up.sql" lines="74-101" />
- `university` spaces for each target university, linked to the correct city/state `parent_id`.
- `university_course_catalog` and `university_course_offerings` records for target courses.
- `visa_route` records for target destination countries.

Ingestion can be:
- a one-off migration with real data,
- admin CRUD in `apps/control`,
- an intelligence/scraping job (future pass).

## 8. Migration plan

1. Update `comparison_type_definitions.config` for system types to add `scope_chain`.
2. Add `region`/`subregion` and `parent_id` support to `ListSpacesPaginated` and the `ListSpaces` handler.
3. Add `state_id`/`city_id` parameters to `ListCOLCities`.
4. Add `parent_id` filter to `ListSpacesByTypePaginated` for universities.
5. Create `university_course_catalog` and `university_course_offerings` tables.
6. Add store/service/resolver for university course offerings.
7. Repoint the `course` resolver to the new offering service.
8. Update `visaRouteResolver` to accept `country_id`.
9. Add `ScopeChain` component and wire it into `apps/main/src/routes/compare/+page.svelte` and `+page.server.ts`.
10. Seed `col_city`, `university`, and `university_course_offering` data for the target examples.

## 9. Out of scope

- Changing the scoring engine or comparison matrix UI beyond the picker.
- Real-time data scraping for cost-of-living or university course data.
- Admin CRUD UI for the new tables (can be added later, but the backend service should support it).

## 10. Open questions

- Should `banks` and `mentors` also be scoped to `country` or `city`? If so, their `scope_chain` can be updated without schema changes.
- Should the `university` step in `courses` allow multi-select, or only one university at a time per flow? The design assumes one university per leaf selection, but the user can change the filter and select a different university.

## 11. References

- Section 30 — Generic Comparison System
- Section 72 — Cost of Living Calculator
- JCE Sub-spec 1 design (`docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md`)
- ADR-072 Cost of Living Calculator
- `apps/main/src/routes/compare/+page.svelte`
- `apps/main/src/routes/compare/+page.server.ts`
- `internal/comparison/resolver/*`
- `internal/spaces/store.go`
