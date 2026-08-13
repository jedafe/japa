-- internal/db/queries/scholarships_intel.sql
-- sqlc queries for the scholarships projection table (intelligence system).
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
--
-- All queries are tenant-scoped (ADR-112 Amendment 2). Every query filters
-- by tenant_id to guarantee data isolation across tenants.

-- name: CreateScholarship :one
INSERT INTO scholarships (
  tenant_id, intel_record_id, guide_id, source_url, source_type, trust_level,
  title, description, country, country_space_id, university,
  degree_level, fields_of_study, eligible_countries, funding,
  deadline, application_url, required_documents, minimum_gpa,
  language_requirements, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25
) RETURNING *;

-- name: GetScholarship :one
SELECT * FROM scholarships WHERE tenant_id = $1 AND id = $2;

-- name: GetScholarshipByIntelRecord :one
SELECT * FROM scholarships WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: GetScholarshipByGuide :one
SELECT * FROM scholarships WHERE tenant_id = $1 AND guide_id = $2;

-- name: GetScholarshipBySourceURL :one
SELECT * FROM scholarships WHERE tenant_id = $1 AND source_url = $2;

-- name: UpdateScholarship :exec
UPDATE scholarships
SET title = $3, description = $4, country = $5, country_space_id = $6, university = $7,
    degree_level = $8, fields_of_study = $9, eligible_countries = $10, funding = $11,
    deadline = $12, application_url = $13, required_documents = $14, minimum_gpa = $15,
    language_requirements = $16, confidence_overall = $17, verification_status = $18,
    freshness_score = $19, override_source = $20, editorial_overrides = $21,
    last_verified_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: UpdateScholarshipFreshness :exec
UPDATE scholarships SET freshness_score = $3, last_verified_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: UpdateScholarshipEditorialOverrides :exec
UPDATE scholarships
SET editorial_overrides = $3,
    override_source = CASE
        WHEN override_source = 'ingestion' THEN 'editorial'
        WHEN override_source = 'editorial' THEN 'editorial'
        ELSE 'mixed'
    END,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListScholarships :many
SELECT * FROM scholarships WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListScholarshipsKeyset :many
-- Keyset (cursor) pagination for the radar feed (spec §731-733). Resumes
-- after the (cursor_created_at, cursor_id) tuple using explicit comparisons
-- (sqlc infers param types reliably from these, unlike row-value
-- comparisons). BUG-006 fix: the prior handler encoded a cursor but never
-- read it, so pagination was offset-only and clients could not resume.
SELECT * FROM scholarships
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: ListScholarshipsBySpaces :many
-- Union query for the followed-countries radar feed (spec §739). Replaces
-- the per-space N+1 loop in HandleRadarFollowed (BUG-008b fix). Returns
-- scholarships whose country_space_id is in the followed set, ordered by
-- created_at desc, capped at limit.
SELECT * FROM scholarships
WHERE tenant_id = $1
  AND country_space_id = ANY($2::uuid[])
ORDER BY created_at DESC
LIMIT $3;

-- name: SearchScholarships :many
SELECT * FROM scholarships
WHERE tenant_id = $1
  AND ($2::text IS NULL OR country = $2)
  AND ($3::text IS NULL OR $3 = ANY(eligible_countries))
  AND ($4::text IS NULL OR degree_level = $4)
  AND ($5::date IS NULL OR deadline >= $5)
  AND ($6::date IS NULL OR deadline <= $6)
  AND ($7::text IS NULL OR $7 = ANY(fields_of_study))
ORDER BY deadline ASC NULLS LAST, confidence_overall DESC
LIMIT $8 OFFSET $9;

-- name: ListScholarshipsBySpace :many
SELECT * FROM scholarships WHERE tenant_id = $1 AND country_space_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- name: ListScholarshipsByCountry :many
SELECT * FROM scholarships WHERE tenant_id = $1 AND country = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4;

-- name: CountScholarshipsByCountry :many
SELECT country, count(*) as count FROM scholarships WHERE tenant_id = $1 GROUP BY country;

-- name: DeleteScholarship :exec
DELETE FROM scholarships WHERE tenant_id = $1 AND id = $2;
