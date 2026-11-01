# University Domain → Source/Intel/Content Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `university` and `university_course` as first-class intelligence domains so crawled sources (university sites, program catalogues) flow source → intel_records → spaces/custom_field_values + course catalog/offerings, with editorial-override respect and review routing.

**Architecture:** Two new entity types registered in the extractor schema registry, each with an embedded LLM prompt template and a domain promoter. The `university` promoter projects onto `spaces` (space_type slug `education`, `level='university'`) + `custom_field_values` (the seeded `university_fields` group), linked to intel records by a new `university_intel_links` table that also carries field-level `editorial_overrides`. The `university_course` promoter resolves/creates `university_course_catalog` rows and upserts `university_course_offerings` keyed by the existing `(course_id, university_id)` unique index. No new Asynq tasks — `intel:extract`/`intel:promote` dispatch generically by entity_type.

**Tech Stack:** Go (chi, sqlc/pgx v5), PostgreSQL, golang-migrate, Asynq.

## Global Constraints

- JEOS: `AGENTS.md` rules apply — never hand-edit `internal/db/gen/` (edit `internal/db/queries/*.sql`, run `make sqlc-gen`); reference section files in code comments; new intel domains must be reflected in JEOS before/with code (Task 6).
- Specs to read first: `docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md`, `.jeos/sections/39-space-types-university.md`, `.jeos/sections/11-job-queue-asynq.md`.
- Migrations: paired `NNNNNN_name.up.sql` / `NNNNNN_name.down.sql` in `migrations/`; next number is **001002** (`.jeos/conventions/database-migration-naming.md` describes a timestamp format that the repo does not use — follow the repo's actual paired-number convention and flag the doc drift in a code comment).
- Tests: `go test ./internal/intelligence/...` — promoter tests are pure unit tests over mock stores (pattern: `internal/intelligence/promoter/scholarship_test.go`); DB-backed tests skip when `JC_DATABASE_URL` is unset.
- University space rows: `space_types.slug = 'education'` (see `universitySpaceTypeSlug` in `internal/universities/handlers.go:37`), `spaces.level = 'university'`, `geo_code` = ISO 3166-1 **alpha-2** (extraction produces ISO-3 `country_array` — convert; `internal/intelligence/stores/country_codes.go` has the lookup).
- Field keys come from the seeded `university_fields` custom_field_group (`migrations/000735_baseline_seed.up.sql:16861-16875`): `initials, official_website_url, contact_email, accreditation_body, world_ranking, country_ranking, founded_year, type, programs_offered, tuition_range, student_population, acceptance_rate, international_support, scholarship_available, application_url` (+ `colleges` from 000873). Resolve field IDs by key via `gen.GetCustomFieldByKey` — never hardcode UUIDs.
- `editorial_overrides` JSONB convention: a set of field names decoded into `map[string]struct{}`; any key present wins over a re-crawl (shared `applyStrPtrField`-family helpers in `internal/intelligence/promoter/helpers.go`).
- Rule 16: no frontend changes needed — `/api/v1/sources/meta` already serves `SubmittableCategories()`/`RegisteredCategories()` to the pickers.
- No new Valkey keys, no new system_settings, no new Asynq task names (Rule 7 satisfied by reusing `intel:promote`).

---

### Task 1: Migration + sqlc queries + codegen

**Files:**
- Create: `migrations/001002_university_intel_links.up.sql`
- Create: `migrations/001002_university_intel_links.down.sql`
- Create: `migrations/001003_course_offering_intel_columns.up.sql`
- Create: `migrations/001003_course_offering_intel_columns.down.sql`
- Create: `internal/db/queries/university_intel.sql`
- Modify: `internal/db/queries/university_courses.sql` (append queries)
- Generated: `internal/db/gen/*.go` (via `make sqlc-gen`)

**Interfaces:**
- Produces: `gen.UniversityIntelLink{SpaceID, IntelRecordID, EditorialOverrides []byte, CreatedAt, UpdatedAt}`; `gen.GetUniversityIntelLinkBySpace`, `gen.GetUniversityIntelLinkByIntelRecord`, `gen.UpsertUniversityIntelLink`, `gen.AddUniversityIntelOverrides`, `gen.GetCourseOfferingByCourseUniversity`, `gen.GetCourseCatalogBySlug`, `gen.CreateCourseOfferingIntel`, `gen.UpdateCourseOfferingIntel`, `gen.CreateCourseCatalogIntel`. `university_course_offerings` gains `intel_record_id` + `editorial_overrides` columns (Task 4 relies on both).

- [ ] **Step 1: Write the link-table migration**

`migrations/001002_university_intel_links.up.sql`:

```sql
-- Links a university space (space_types.slug='education', level='university')
-- to the intel_record that created/last refreshed it, and carries the
-- field-level editorial_overrides set the promoter respects on re-crawl.
-- Mirrors the editorial_overrides convention on scholarships et al. without
-- adding intel plumbing columns to the shared spaces table.
CREATE TABLE public.university_intel_links (
    space_id            uuid PRIMARY KEY REFERENCES public.spaces(id) ON DELETE CASCADE,
    intel_record_id     uuid NOT NULL REFERENCES public.intel_records(id) ON DELETE CASCADE,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at          timestamptz DEFAULT now() NOT NULL,
    updated_at          timestamptz DEFAULT now() NOT NULL
);

CREATE INDEX idx_university_intel_links_record
    ON public.university_intel_links (intel_record_id);
```

`migrations/001002_university_intel_links.down.sql`:

```sql
DROP TABLE IF EXISTS public.university_intel_links;
```

- [ ] **Step 2: Write the offerings columns migration**

`migrations/001003_course_offering_intel_columns.up.sql`:

```sql
-- intel_record_id: provenance for the promoter + radar. editorial_overrides:
-- field-level set the promoter skips on re-crawl (human edit wins).
ALTER TABLE public.university_course_offerings
    ADD COLUMN intel_record_id uuid REFERENCES public.intel_records(id) ON DELETE SET NULL,
    ADD COLUMN editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL;

CREATE INDEX idx_course_offerings_intel_record
    ON public.university_course_offerings (intel_record_id)
    WHERE intel_record_id IS NOT NULL;
```

`migrations/001003_course_offering_intel_columns.down.sql`:

```sql
DROP INDEX IF EXISTS public.idx_course_offerings_intel_record;
ALTER TABLE public.university_course_offerings
    DROP COLUMN IF EXISTS intel_record_id,
    DROP COLUMN IF EXISTS editorial_overrides;
```

- [ ] **Step 3: Write `internal/db/queries/university_intel.sql`**

```sql
-- name: GetUniversityIntelLinkBySpace :one
SELECT space_id, intel_record_id, editorial_overrides, created_at, updated_at
FROM university_intel_links
WHERE space_id = @space_id;

-- name: GetUniversityIntelLinkByIntelRecord :one
SELECT space_id, intel_record_id, editorial_overrides, created_at, updated_at
FROM university_intel_links
WHERE intel_record_id = @intel_record_id;

-- name: UpsertUniversityIntelLink :one
-- Create the link or repoint it at the newest promoted record. Overrides are
-- preserved on conflict — they are human state, not crawl state.
INSERT INTO university_intel_links (space_id, intel_record_id)
VALUES (@space_id, @intel_record_id)
ON CONFLICT (space_id) DO UPDATE
    SET intel_record_id = EXCLUDED.intel_record_id,
        updated_at = now()
RETURNING space_id, intel_record_id, editorial_overrides, created_at, updated_at;

-- name: AddUniversityIntelOverrides :exec
-- Merge field keys into the override set (admin-edit marking, Task 5).
UPDATE university_intel_links
SET editorial_overrides = editorial_overrides || @overrides::jsonb,
    updated_at = now()
WHERE space_id = @space_id;

-- name: GetUniversitySpaceBySlug :one
-- University = spaces row on the 'education' space type with level='university'.
SELECT s.id, s.name, s.slug, s.geo_code, s.parent_id, s.status
FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE st.slug = 'education' AND s.level = 'university' AND s.slug = @slug
LIMIT 1;
```

- [ ] **Step 4: Append course-offering queries to `internal/db/queries/university_courses.sql`**

```sql
-- name: GetCourseOfferingByCourseUniversity :one
SELECT id, course_id, university_id, slug, tuition, currency, duration_years,
       study_mode, intake_months, entry_requirements, accreditation,
       is_published, intel_record_id, editorial_overrides, created_at, updated_at
FROM university_course_offerings
WHERE course_id = @course_id AND university_id = @university_id
  AND deleted_at IS NULL
LIMIT 1;

-- name: GetCourseCatalogBySlug :one
SELECT id, title, slug, description, degree_type, department, field_of_study,
       language, default_duration_years, is_published, created_at, updated_at
FROM university_course_catalog
WHERE slug = @slug AND deleted_at IS NULL
LIMIT 1;

-- name: CreateCourseOfferingIntel :one
INSERT INTO university_course_offerings (
    course_id, university_id, slug, tuition, currency, duration_years,
    study_mode, intake_months, entry_requirements, accreditation,
    is_published, intel_record_id
) VALUES (
    @course_id, @university_id, @slug, @tuition, @currency, @duration_years,
    @study_mode, @intake_months, @entry_requirements, @accreditation,
    @is_published, @intel_record_id
)
RETURNING id;

-- name: UpdateCourseOfferingIntel :exec
-- COALESCE keeps existing values when the extraction yields NULL; the
-- promoter pre-resolves overrides so protected fields keep existing values
-- in the params it passes.
UPDATE university_course_offerings
SET tuition = @tuition,
    currency = @currency,
    duration_years = @duration_years,
    study_mode = @study_mode,
    intake_months = @intake_months,
    entry_requirements = @entry_requirements,
    accreditation = @accreditation,
    is_published = @is_published,
    intel_record_id = @intel_record_id,
    updated_at = now()
WHERE id = @id;

-- name: CreateCourseCatalogIntel :one
-- Minimal catalog row for a course the pipeline has never seen; curated
-- fields (description, department) stay NULL for editorial fill-in.
INSERT INTO university_course_catalog (
    title, slug, degree_type, field_of_study, language, default_duration_years,
    is_published
) VALUES (
    @title, @slug, @degree_type, @field_of_study, @language, @default_duration_years,
    true
)
RETURNING id;
```

- [ ] **Step 5: Run migrations + regenerate sqlc**

Run: `make migrate-up` (or the project's local migrate target) then `make sqlc-gen`
Expected: both migrations apply; `internal/db/gen/university_intel.sql.go` appears; `university_courses.sql.go` gains the new queries and `UniversityCourseOffering` gains `IntelRecordID`/`EditorialOverrides`.

- [ ] **Step 6: Commit**

```bash
git add migrations/001002_* migrations/001003_* internal/db/queries/university_intel.sql internal/db/queries/university_courses.sql internal/db/gen/
git commit -m "feat(intel): university intel link table + course offering intel columns"
```

---

### Task 2: Schemas, registry, prompts, and source taxonomy

**Files:**
- Create: `internal/intelligence/extractor/schemas/university.go`
- Create: `internal/intelligence/extractor/schemas/university_course.go`
- Create: `internal/intelligence/extractor/prompts/university.txt`
- Create: `internal/intelligence/extractor/prompts/university_course.txt`
- Modify: `internal/intelligence/extractor/schemas/registry.go`
- Modify: `internal/intelligence/extractor/extractor.go` (~lines 353-430: embed block + `promptTemplates` map)
- Modify: `internal/intelligence/sources/sources.go` (category consts + `submittableCategories`)
- Modify: `internal/intelligence/sources/category_map.json`
- Test: `internal/intelligence/extractor/schemas/registry_test.go`

**Interfaces:**
- Produces: `schemas.EntityTypeUniversity = "university"`, `schemas.EntityTypeUniversityCourse = "university_course"`; `schemas.University`, `schemas.UniversityCourse` EntitySchemas; `sources.CategoryUniversity`, `sources.CategoryUniversityCourse`. Extract dispatch resolves both automatically via `resolveIntelEntityTypes` (`internal/worker/intelligence.go:1804`) — category or `provider_config.entity_types`.

- [ ] **Step 1: Write the failing registry test**

Append to `internal/intelligence/extractor/schemas/registry_test.go`:

```go
func TestUniversityEntityTypesRegistered(t *testing.T) {
	for _, et := range []string{"university", "university_course"} {
		s, ok := ForEntityType(et)
		if !ok {
			t.Fatalf("ForEntityType(%q) not registered", et)
		}
		if s.CurateOnly {
			t.Fatalf("ForEntityType(%q) must not be curate-only", et)
		}
		if len(s.Fields) == 0 {
			t.Fatalf("ForEntityType(%q) has no fields", et)
		}
	}

	if got := CategoryToEntityType("university"); got != "university" {
		t.Fatalf("CategoryToEntityType(university) = %q", got)
	}
	if got := CategoryToEntityType("education"); got != "university" {
		t.Fatalf("CategoryToEntityType(education) = %q", got)
	}
	if got := CategoryToEntityType("university_course"); got != "university_course" {
		t.Fatalf("CategoryToEntityType(university_course) = %q", got)
	}
}
```

Run: `go test ./internal/intelligence/extractor/schemas/ -run TestUniversityEntityTypesRegistered -v`
Expected: FAIL — entity types not registered.

- [ ] **Step 2: Create `internal/intelligence/extractor/schemas/university.go`**

```go
package schemas

// University is the entity schema for the university domain.
// Projection: spaces row (space_type 'education', level='university') +
// custom_field_values on the seeded university_fields group (§39). Field
// names mirror the group's custom-field keys so the promoter maps 1:1.
var University = EntitySchema{
	EntityType: EntityTypeUniversity,
	Fields: []FieldSpec{
		{Name: "name", Type: "string", Required: true},
		{Name: "slug", Type: "string", Required: false}, // derived from name when absent
		{Name: "initials", Type: "string", Required: false},
		{Name: "country", Type: "country_array", Required: true}, // ISO-3, 1-element
		{Name: "city", Type: "string", Required: false},
		{Name: "official_website_url", Type: "string", Required: false},
		{Name: "contact_email", Type: "string", Required: false},
		{Name: "accreditation_body", Type: "string", Required: false},
		{Name: "world_ranking", Type: "numeric", Required: false},
		{Name: "country_ranking", Type: "numeric", Required: false},
		{Name: "founded_year", Type: "numeric", Required: false},
		{Name: "type", Type: "string", Required: false, EnumValues: []string{"public", "private"}},
		{Name: "programs_offered", Type: "text_array", Required: false},
		{Name: "tuition_range", Type: "string", Required: false, EnumValues: []string{"free", "<5k", "5k-15k", "15k-30k", ">30k"}},
		{Name: "student_population", Type: "numeric", Required: false},
		{Name: "acceptance_rate", Type: "numeric", Required: false},
		{Name: "international_support", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "scholarship_available", Type: "string", Required: false, EnumValues: []string{"true", "false"}},
		{Name: "application_url", Type: "string", Required: false},
		{Name: "description", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
```

- [ ] **Step 3: Create `internal/intelligence/extractor/schemas/university_course.go`**

```go
package schemas

// UniversityCourse is the entity schema for course/program offerings.
// Projection: university_course_offerings (+ university_course_catalog
// resolve-or-create). One record = one offering at one university.
var UniversityCourse = EntitySchema{
	EntityType: EntityTypeUniversityCourse,
	Fields: []FieldSpec{
		{Name: "course_title", Type: "string", Required: true},
		{Name: "university_name", Type: "string", Required: true},
		{Name: "university_slug", Type: "string", Required: false}, // fast-path resolution
		{Name: "country", Type: "country_array", Required: false},  // disambiguates university_name
		{Name: "degree_type", Type: "string", Required: false, EnumValues: []string{"bachelors", "masters", "phd", "postdoc", "diploma", "certificate", "associate"}},
		{Name: "field_of_study", Type: "string", Required: false},
		{Name: "department", Type: "string", Required: false},
		{Name: "language", Type: "string", Required: false},
		{Name: "duration_years", Type: "numeric", Required: false},
		{Name: "tuition", Type: "numeric", Required: false},
		{Name: "currency", Type: "string", Required: false},
		{Name: "study_mode", Type: "string", Required: false, EnumValues: []string{"full_time", "part_time", "online", "hybrid"}},
		{Name: "intake_months", Type: "text_array", Required: false},
		{Name: "entry_requirements", Type: "string", Required: false},
		{Name: "accreditation", Type: "string", Required: false},
		{Name: "source_url", Type: "string", Required: true},
	},
}
```

- [ ] **Step 4: Register in `internal/intelligence/extractor/schemas/registry.go`**

Add to the const block:

```go
	EntityTypeUniversity       = "university"
	EntityTypeUniversityCourse = "university_course"
```

Add to `registry`:

```go
	EntityTypeUniversity:       University,
	EntityTypeUniversityCourse: UniversityCourse,
```

Add to `categoryEntityTypes`:

```go
	"university":         EntityTypeUniversity,
	"education":          EntityTypeUniversity,
	"university_course":  EntityTypeUniversityCourse,
	"course_offering":    EntityTypeUniversityCourse,
	"program":            EntityTypeUniversityCourse,
```

Add `"university"` and `"university_course"` (sorted) to `RegisteredCategories()`.

- [ ] **Step 5: Write the prompt templates**

`internal/intelligence/extractor/prompts/university.txt` — follow the structure of `prompts/scholarship.txt` (extraction contract, field list with types/enums, `source_url` required, output JSON of `{field: {value, confidence, evidence}}`). Extract a single university entity per page: identity (name/initials/country/city), contact (website/email/application_url), and the ranking/admissions fields matching the schema enum values exactly (`type` → `public|private`, `tuition_range` → `free|<5k|5k-15k|15k-30k|>30k`).

`internal/intelligence/extractor/prompts/university_course.txt` — extract one course offering: `course_title`, `university_name` (verbatim as on the page), `degree_type` enum, `tuition` numeric + `currency` ISO-4217, `duration_years` numeric, `study_mode` enum, `intake_months` as month names array, `source_url` required.

- [ ] **Step 6: Wire the embeds in `internal/intelligence/extractor/extractor.go`**

After the existing embed block (~line 401), add:

```go
//go:embed prompts/university.txt
var universityPrompt string

//go:embed prompts/university_course.txt
var universityCoursePrompt string
```

Add to `promptTemplates`:

```go
	schemas.EntityTypeUniversity:       universityPrompt,
	schemas.EntityTypeUniversityCourse: universityCoursePrompt,
```

- [ ] **Step 7: Update the source taxonomy in `internal/intelligence/sources/sources.go`**

Add to the `Category` consts:

```go
	CategoryUniversity       Category = "university"
	CategoryUniversityCourse Category = "university_course"
```

Add to `submittableCategories` (both domains reach content end-to-end):

```go
	CategoryUniversity:       {},
	CategoryUniversityCourse: {},
```

- [ ] **Step 8: Add `category_map.json` entries**

```json
  "university": {
    "entity_types": ["university"],
    "methods": ["static", "jsonld", "render", "sitemap", "spider"],
    "pipeline": "extract",
    "topics": ["university", "admissions", "ranking", "tuition", "programs", "faculties"]
  },
  "university_course": {
    "entity_types": ["university_course"],
    "methods": ["static", "jsonld", "render", "sitemap", "spider"],
    "pipeline": "extract",
    "topics": ["course", "program", "degree", "masters", "bachelors", "tuition", "intake"]
  }
```

- [ ] **Step 9: Run tests**

Run: `go test ./internal/intelligence/extractor/... ./internal/intelligence/sources/... && go build ./internal/...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/intelligence/extractor internal/intelligence/sources
git commit -m "feat(intel): register university + university_course extraction domains"
```

---

### Task 3: `stores.Store` passthrough methods

**Files:**
- Modify: `internal/intelligence/stores/stores.go`
- Modify: `internal/intelligence/stores/stores_test.go`

**Interfaces:**
- Consumes: gen queries from Task 1.
- Produces (consumed by Task 4 + server adapters):
  - `GetUniversitySpaceBySlug(ctx, slug string) (gen.GetUniversitySpaceBySlugRow, error)`
  - `GetSpaceTypeBySlug(ctx, slug string) (gen.SpaceType, error)`
  - `GetUniversityIntelLinkBySpace(ctx, spaceID pgtype.UUID) (gen.UniversityIntelLink, error)`
  - `GetUniversityIntelLinkByIntelRecord(ctx, recordID pgtype.UUID) (gen.UniversityIntelLink, error)`
  - `UpsertUniversityIntelLink(ctx, arg gen.UpsertUniversityIntelLinkParams) (gen.UniversityIntelLink, error)`
  - `AddUniversityIntelOverrides(ctx, arg gen.AddUniversityIntelOverridesParams) error`
  - `CreateSpace(ctx, arg gen.CreateSpaceParams) (gen.CreateSpaceRow, error)`
  - `UpsertCustomFieldValue(ctx, arg gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error)`
  - `GetCustomFieldByKey(ctx, key string) (gen.GetCustomFieldByKeyRow, error)`
  - `ListCustomFieldValuesWithDefsByEntity(ctx, arg gen.ListCustomFieldValuesWithDefsByEntityParams) ([]gen.ListCustomFieldValuesWithDefsByEntityRow, error)`
  - `GetCustomFieldGroupBySlug(ctx, slug string) (gen.CustomFieldGroup, error)`
  - `GetCustomFieldByGroupAndKey(ctx, arg gen.GetCustomFieldByGroupAndKeyParams) (gen.CustomField, error)`
  - `GetSpaceByID(ctx, id pgtype.UUID) (gen.GetSpaceByIDRow, error)`
  - `UpdateSpace(ctx, arg gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error)`
  - `CountryCodeAlpha2(raw string) string` (exported wrapper over `countryCodeCandidates`)
  - `GetCourseOfferingByCourseUniversity(ctx, arg gen.GetCourseOfferingByCourseUniversityParams) (gen.GetCourseOfferingByCourseUniversityRow, error)`
  - `GetCourseCatalogBySlug(ctx, slug string) (gen.UniversityCourseCatalog, error)`
  - `CreateCourseCatalogIntel(ctx, arg gen.CreateCourseCatalogIntelParams) (pgtype.UUID, error)`
  - `CreateCourseOfferingIntel(ctx, arg gen.CreateCourseOfferingIntelParams) (pgtype.UUID, error)`
  - `UpdateCourseOfferingIntel(ctx, arg gen.UpdateCourseOfferingIntelParams) error`

- [ ] **Step 1: Add passthroughs to `stores.go`**

Every method is a one-line delegate to `s.q.<QueryName>(ctx, arg)` inside `s.WithTx`-compatible form — copy the exact shape of the existing methods at `stores.go:74-500` (e.g. `GetCommunityOrganisationByIntelRecord`). Verify each gen signature in `internal/db/gen/` before writing the delegate.

- [ ] **Step 2: Build + commit**

Run: `go build ./internal/intelligence/stores/ && go vet ./internal/intelligence/stores/`

```bash
git add internal/intelligence/stores/stores.go
git commit -m "feat(intel): university domain store passthroughs"
```

---

### Task 4: `UniversityPromoter` + `UniversityCoursePromoter`

**Files:**
- Create: `internal/intelligence/promoter/university.go`
- Create: `internal/intelligence/promoter/university_course.go`
- Create: `internal/intelligence/promoter/university_test.go`
- Create: `internal/intelligence/promoter/university_course_test.go`
- Modify: `internal/intelligence/handlers/../..` — none; wiring lives in server (Task 5).

**Interfaces:**
- Consumes: store methods from Task 3; `promoteDomain`/`domainPromoteOps` skeleton + `applyStrPtrField` family + `parseExtractionShared` + `resolveAuthorID` from `internal/intelligence/promoter/helpers.go`; `universities.DeriveInitials` (`internal/universities/initials.go`) for fallback initials; ISO-3→alpha-2 via `stores` country-code helper.
- Produces: `NewUniversityPromoter(store UniversityStore, authorID pgtype.UUID, opts ...UniversityPromoterOption) *UniversityPromoter` (`Entity() == "university"`); `NewUniversityCoursePromoter(store UniversityCourseStore, authorID pgtype.UUID, opts ...UniversityCoursePromoterOption)`.

**Design notes to encode:**

- Store interfaces follow `community_org.go:28-57`: embed `PersonaResolver`, `GetIntelRecord`, `GetIntelSource`, `UpdateIntelRecordStatus`, `CreateIntelReviewLog`, `WithTx(ctx, fn func(TxStore) error)`; TxStore carries only the write methods the create/update paths call.
- `Promote` uses `promoteDomain` with `E = gen.UniversityIntelLink` (university) / `E = gen.GetCourseOfferingByCourseUniversityRow` is NOT the getExisting input — for courses getExisting looks up the offering via the resolved (course_id, university_id) pair *inside* the path, so `E` can be a small struct `{Link gen.UniversityIntelLink}`… simplest: for the course promoter skip `promoteDomain` and write `Promote` directly (record → resolve university → resolve/create catalog → getExisting offering → create or update path), mirroring the skeleton's flow manually since there is no per-record projection row to fetch up front.
- Create path (university): build slug via `slugify(name)`; `GetUniversitySpaceBySlug` hit without a link = **adopt** (create link, run update path — never duplicate a university an admin seeded). Miss → `CreateSpace` with `SpaceTypeID` (from `GetSpaceTypeBySlug("education")`), `Level: "university"`, `Visibility: "public"`, perms `members`/`open`, `Status: "active"`, `GeoCode` = alpha-2; then upsert CFVs for every extracted field that maps to a `university_fields` key; `UpsertUniversityIntelLink`; review-log `created`.
- Update path (university): diff each extracted field against current CFVs (`ListFieldValuesByEntity` keyed by field key) and `name`/`geo_code` against the space; skip any key in `link.EditorialOverrides`; write only changed CFVs; review-log `automatic re-crawl update` with `changed` list; action `updated` or `verified`.
- Geo: extract `country` ISO-3 → alpha-2 for `spaces.geo_code`; `GetCountrySpace` for `parent_id` (nil-safe). Unresolvable country on create → `ErrRouteToReview`.
- Course promoter: resolve university by `university_slug` → `university_name`+country fuzzy match (`GetUniversitySpaceBySlug(slugify(name))`, then a name/initials match within the country's universities); unresolved → `ErrRouteToReview`. Resolve catalog by `slugify(course_title)`+`degree_type`; miss → `CreateCourseCatalogIntel`. Offering slug = `{catalog.slug}-{university.slug}` (deterministic → re-crawls converge). Upsert on `(course_id, university_id)`; update path respects `editorial_overrides` field names: `tuition, currency, duration_years, study_mode, intake_months, entry_requirements, accreditation`.
- Both promoters stamp `is_published = record status promoted through the gate` (records only reach promote post-approval, so `true`), set `intel_record_id`, and write `CreateIntelReviewLog` entries (`auto_ingestion` actor = `authorID`).

- [ ] **Step 1: Write failing tests — university create path**

`internal/intelligence/promoter/university_test.go` — mock `UniversityStore` (map-backed, same shape as `community_org`/`scholarship` test mocks):

```go
func TestUniversityPromote_CreateBuildsSpaceCFVsAndLink(t *testing.T) {
	store := newMockUniversityStore()
	p := NewUniversityPromoter(store, testAuthorID)

	rec := store.seedRecord(t, "university", map[string]any{
		"name": "Test University", "country": []string{"CAN"},
		"official_website_url": "https://testu.ca", "type": "public",
		"world_ranking": 42, "source_url": "https://testu.ca/about",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionCreated {
		t.Fatalf("Action = %q, want created", res.Action)
	}
	space := store.spaces[res.DomainID]
	if space == nil || space.Level != "university" || space.Slug != "test-university" {
		t.Fatalf("unexpected space: %+v", space)
	}
	if space.GeoCode == nil || *space.GeoCode != "CA" {
		t.Fatalf("GeoCode = %v, want CA", space.GeoCode)
	}
	if got := store.cfvValue(res.DomainID, "world_ranking"); got != "42" {
		t.Fatalf("world_ranking CFV = %q", got)
	}
	link := store.links[res.DomainID]
	if link == nil || link.IntelRecordID != rec.ID {
		t.Fatal("link row missing or wrong intel_record_id")
	}
}
```

Also: `TestUniversityPromote_UpdateRespectsEditorialOverrides` (seed link with `editorial_overrides: {"world_ranking": true}`, change extraction value, assert CFV untouched, `Action == updated` for another field or `verified`); `TestUniversityPromote_AdoptsUnlinkedSpaceBySlug`; `TestUniversityPromote_NoCountryRoutesToReview` (`errors.Is(err, ErrRouteToReview)`).

- [ ] **Step 2: Implement `promoter/university.go`**

Write the promoter per the design notes. Field→CFV mapping is 1:1 with schema names except `name`/`country`/`city`/`description`/`slug` which map to space columns; `type` values stay lowercase (`public`/`private` match seeded options); booleans arrive as `"true"/"false"` strings → `value` JSON `{"value": true}` for CFV field_type `boolean`; numbers → `{"value": <n>}`; multiselect `programs_offered` → `{"values": [...]}`; url/email/text → `{"value": "..."}`. Match the CFV `value`/`values` JSON envelope `service_fields.go:1700` expects.

- [ ] **Step 3: Write failing tests — course promoter**

`university_course_test.go`:

```go
func TestCoursePromote_CreatesCatalogAndOffering(t *testing.T) { /* seed university space; assert catalog row + offering with slug "msc-computer-science-test-university", intel_record_id stamped */ }
func TestCoursePromote_UpdatesExistingOffering(t *testing.T)    { /* changed tuition → Action updated, ChangedFields ["tuition"] */ }
func TestCoursePromote_OverrideSkipsField(t *testing.T)         { /* editorial_overrides {"tuition"} → tuition untouched */ }
func TestCoursePromote_UnknownUniversityRoutesToReview(t *testing.T) { /* errors.Is ErrRouteToReview */ }
```

- [ ] **Step 4: Implement `promoter/university_course.go`**

Per design notes. Currency validates as 3-letter ISO before store; `intake_months` text[] → `[]string` lowercased month names.

- [ ] **Step 5: Run tests + commit**

Run: `go test ./internal/intelligence/promoter/ -run 'University|Course' -v`
Expected: PASS.

```bash
git add internal/intelligence/promoter/university*.go
git commit -m "feat(intel): university + course offering promoters"
```

---

### Task 5: Server wiring + editorial-override marking on admin edits

**Files:**
- Modify: `internal/server/intel_adapters.go` (two adapters, after `intelCommunityOrgPromoterStoreAdapter` ~line 617)
- Modify: `internal/server/server.go` (`intelPromoters` map, ~line 2849)
- Modify: `internal/universities/handlers.go` (`UpdateUniversity` ~line 918 + the CFV update path) — mark overrides so admin edits win. Two marking points: `UpdateUniversity` (space columns + initials) and `customfields.SetFieldValue` (PUT /custom-field-values — the generic path the university admin form uses for ranked/meta fields) via an optional injected `MarkIntelOverride`-style dependency
- Test: `go build ./internal/server/`

**Interfaces:**
- Consumes: `promoter.NewUniversityPromoter`, `promoter.NewUniversityCoursePromoter`, `stores.Store` methods from Task 3, `worker.IntelPromoterPort` + `intelPromoterPortAdapter` (existing pattern).
- Produces: `intelUniversityPromoterStoreAdapter`, `intelUniversityCoursePromoterStoreAdapter`; `schemas.EntityTypeUniversity` + `schemas.EntityTypeUniversityCourse` entries in `intelPromoters`.

- [ ] **Step 1: Write the adapters**

In `intel_adapters.go`, mirror `intelCommunityOrgPromoterStoreAdapter` (line ~615): struct wrapping `*stores.Store`, `WithTx` delegating to `s.WithTx(ctx, func(tx *stores.Store) error { return fn(tx) })`, and each `UniversityStore`/`UniversityCourseStore` method delegating to the embedded store (most resolve automatically via embedding — only `WithTx` needs an explicit body when the TxStore types differ).

- [ ] **Step 2: Register in `server.go`**

```go
		schemas.EntityTypeUniversity: &intelPromoterPortAdapter{
			prom: promoter.NewUniversityPromoter(&intelUniversityPromoterStoreAdapter{Store: intelStores}, intelDefaultAuthorID, promoter.WithUniversityAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeUniversityCourse: &intelPromoterPortAdapter{
			prom: promoter.NewUniversityCoursePromoter(&intelUniversityCoursePromoterStoreAdapter{Store: intelStores}, intelDefaultAuthorID, promoter.WithUniversityCourseAIModel(intelExtractionModel)),
		},
```

- [ ] **Step 3: Override marking on admin edits**

In `internal/universities/handlers.go`: `UpdateUniversity` already knows which fields the admin touched (space columns + CFV keys in the request). After a successful update, when a `university_intel_links` row exists for the space, call `AddUniversityIntelOverrides(ctx, {SpaceID, Overrides})` with the touched keys as a JSON object `{"name":true,"world_ranking":true,...}`. Inject a narrow `AddUniversityIntelOverrides(ctx, arg gen.AddUniversityIntelOverridesParams) error` dependency on the handlers' service (same adapter style as `CustomFieldsService`). Admin edits to `custom_field_values` through the *generic* CF endpoints are out of scope — the university surface is the curated path.

- [ ] **Step 4: Build + smoke**

Run: `go build ./internal/... && go test ./internal/intelligence/... ./internal/universities/...`

- [ ] **Step 5: Commit**

```bash
git add internal/server internal/universities
git commit -m "feat(intel): wire university promoters + admin-edit override marking"
```

---

### Task 6 (optional, recommended): Radar surfacing for course offerings + JEOS/docs

**Files:**
- Modify: `internal/db/queries/university_courses.sql` (`ListCourseOfferingsIntelKeyset`)
- Modify: `internal/intelligence/handlers/radar.go` (converter + `radarFetch` branch + `radarTypeRank` + filter label)
- Modify: `.jeos/sections/39-space-types-university.md` (intelligence ingestion subsection), `docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md` (domain list)

- [ ] **Step 1: Keyset query + gen + converter**

`ListCourseOfferingsIntelKeyset` — same shape as `ListIntelJobsKeyset` (`intelligence_domains.sql:46`): `(ts, id)` cursor, join catalog for title. `courseOfferingToRadarItem` → `Type: "course_offering"`, Title = `{course.title} — {university.name}`, FieldValues = tuition/currency/study_mode/intakes.

- [ ] **Step 2: Radar fetch branches**

`add("course_offering", ...)` in `radarFeedDomains` + `radarFollowedDomains` (scoped by `university_id` IN followed space IDs — university follows already exist), filter case `"course_offering", "course"`, rank in `radarTypeRank`.

- [ ] **Step 3: JEOS updates**

Add a "Source-of-truth ingestion (intelligence pipeline)" subsection to §39 noting the `university`/`university_course` entity types, the `university_intel_links` table, and the override semantics; add both entity types to the intelligence design doc's domain list. No §11 changes (no new task names); no system-settings changes.

- [ ] **Step 4: Test + commit**

```bash
go test ./internal/intelligence/handlers/ -v
git add internal/db/queries internal/db/gen internal/intelligence/handlers .jeos docs
git commit -m "feat(intel): radar surfacing for course offerings + JEOS updates"
```

---

### Task 7: End-to-end verification

- [ ] **Step 1:** `go build ./... && go test ./internal/intelligence/...`
- [ ] **Step 2:** Seed a test source: `category='university'`, `source_kind='institution_site'`, `crawl_method='static'` pointing at a real university about page; run `intel:fetch` → `intel:extract` → approve → `intel:promote`; assert space + CFVs + link row.
- [ ] **Step 3:** Seed a `university_course` source (or `provider_config.entity_types=["university","university_course"]` on the same site with a programs page via `spider`); assert catalog + offering rows.
- [ ] **Step 4:** Admin-edit a field in the university panel; re-run the crawl; assert the edited field is unchanged and the override was logged.

## Follow-ups (out of scope)

- Hipolabs API as a `crawl_method=api` `university` source (§39 already uses it for the CLI seeder — natural first real source).
- `space:auto_populate` enqueue on promoter-created spaces (Wikidata enrichment for fields intel doesn't cover) — needs an enqueuer port on the promoter or a post-promote worker hook.
- Radar surfacing for university space updates themselves (low signal — covered by offerings).
