-- internal/db/queries/scholarships_intel.sql
-- sqlc queries for the scholarships projection table (intelligence system).
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
--
-- All queries are platform-scoped (ADR-112 Amendment 2). Every query filters
-- by the id to guarantee data isolation across platforms.

-- name: CreateScholarship :one
INSERT INTO scholarships (intel_record_id, guide_id, source_url, source_type, trust_level, title, description, country, country_space_id, university, degree_level, fields_of_study, eligible_countries, funding, deadline, application_url, required_documents, minimum_gpa, language_requirements, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24) RETURNING *;

-- name: GetScholarship :one
SELECT * FROM scholarships WHERE id = $1;

-- name: GetScholarshipByIntelRecord :one
SELECT * FROM scholarships WHERE intel_record_id = $1;

-- name: GetScholarshipByGuide :one
SELECT * FROM scholarships WHERE guide_id = $1;

-- name: GetScholarshipBySourceURL :one
SELECT * FROM scholarships WHERE source_url = $1;

-- name: UpdateScholarship :exec
UPDATE scholarships
SET title = $2, description = $3, country = $4, country_space_id = $5, university = $6,
 degree_level = $7, fields_of_study = $8, eligible_countries = $9, funding = $10,
 deadline = $11, application_url = $12, required_documents = $13, minimum_gpa = $14,
 language_requirements = $15, confidence_overall = $16, verification_status = $17,
 freshness_score = $18, override_source = $19, editorial_overrides = $20,
 source_url = $21, source_type = $22, trust_level = $23,
 guide_id = COALESCE($24, guide_id),
 last_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: UpdateScholarshipFreshness :exec
UPDATE scholarships SET freshness_score = $2, last_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: UpdateScholarshipEditorialOverrides :exec
UPDATE scholarships
SET editorial_overrides = $2,
 override_source = CASE
 WHEN override_source = 'ingestion' THEN 'editorial'
 WHEN override_source = 'editorial' THEN 'editorial'
 ELSE 'mixed'
 END,
 updated_at = now()
WHERE id = $1;

-- name: ListScholarships :many
SELECT * FROM scholarships WHERE true ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: AdminListScholarships :many
-- Admin management list with optional text search (q) and verification_status
-- filter. q matches title, university, or country case-insensitively. Drives
-- the /admin/scholarships list endpoint (Section 73).
SELECT * FROM scholarships
WHERE ($1::text IS NULL OR $1 = '' OR title ILIKE '%' || $1 || '%' OR university ILIKE '%' || $1 || '%' OR country ILIKE '%' || $1 || '%')
 AND ($2::text IS NULL OR $2 = '' OR verification_status = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: AdminCountScholarships :one
-- Total row count for the admin management list with the same q + status
-- filter as AdminListScholarships, so the admin panel can compute total pages.
SELECT COUNT(*) FROM scholarships
WHERE ($1::text IS NULL OR $1 = '' OR title ILIKE '%' || $1 || '%' OR university ILIKE '%' || $1 || '%' OR country ILIKE '%' || $1 || '%')
 AND ($2::text IS NULL OR $2 = '' OR verification_status = $2);

-- name: ListScholarshipsKeyset :many
-- Keyset (cursor) pagination for the radar feed (spec §731-733). Resumes
-- after the (cursor_created_at, cursor_id) tuple using explicit comparisons
-- (sqlc infers param types reliably from these, unlike row-value
-- comparisons). BUG-006 fix: the prior handler encoded a cursor but never
-- read it, so pagination was offset-only and clients could not resume.
-- The optional geo predicates ($3 space, $4 ISO country) are part of the
-- keyset query so a cursor resumes a geo-filtered feed without dropping
-- the filter — previously ?cursor= bypassed the space/country branches.
-- $4 also matches via spaces.geo_code so callers can pass either the
-- scholarship country vocabulary or a country space's geo_code.
SELECT * FROM scholarships
WHERE ($1::timestamptz IS NULL
 OR scholarships.created_at < $1
 OR (scholarships.created_at = $1 AND scholarships.id < $2))
 AND ($3::uuid IS NULL OR scholarships.country_space_id = $3)
 AND (NULLIF($4::text, '') IS NULL OR scholarships.country = $4
      OR scholarships.country_space_id IN (SELECT spaces.id FROM spaces WHERE spaces.geo_code = $4))
 AND ($5::timestamptz IS NULL OR scholarships.created_at >= $5)
ORDER BY scholarships.created_at DESC, scholarships.id DESC
LIMIT $6;

-- name: ListScholarshipsBySpaces :many
-- Union query for the followed-countries radar feed (spec §739). Replaces
-- the per-space N+1 loop in HandleRadarFollowed (BUG-008b fix). Returns
-- scholarships whose country_space_id is in the followed set, ordered by
-- created_at desc, capped at limit. The optional keyset cursor ($2,$3)
-- lets the merged multi-domain feed paginate — for offset pages the
-- handler passes LIMIT = offset+limit and slices in memory.
SELECT * FROM scholarships
WHERE country_space_id = ANY($1::uuid[])
 AND ($2::timestamptz IS NULL
 OR created_at < $2
 OR (created_at = $2 AND id < $3))
 AND ($4::timestamptz IS NULL OR created_at >= $4)
ORDER BY created_at DESC, id DESC
LIMIT $5;

-- name: SearchScholarships :many
-- Optional text filters use NULLIF($n,'') because sqlc types $n::text as a
-- non-nullable string — an unset filter arrives as '', not NULL.
-- $6 fields_of_study matches ANY of the CSV elements (overlap), $7 is
-- min_confidence, $8 is the q text search, $9 is funding_tuition against
-- the funding JSONB's tuition flag. All filters run in SQL so LIMIT/OFFSET
-- pages are filled correctly — the previous implementation post-filtered
-- in memory after LIMIT, which under-filled pages and dropped matches.
SELECT * FROM scholarships
WHERE (NULLIF($1::text, '') IS NULL OR country = $1)
 AND (NULLIF($2::text, '') IS NULL OR $2 = ANY(eligible_countries))
 AND (NULLIF($3::text, '') IS NULL OR degree_level = $3)
 AND ($4::date IS NULL OR deadline >= $4)
 AND ($5::date IS NULL OR deadline <= $5)
 AND ($6::text[] IS NULL OR fields_of_study && $6)
 AND confidence_overall >= $7
 AND (NULLIF($8::text, '') IS NULL
  OR title ILIKE '%' || $8 || '%'
  OR description ILIKE '%' || $8 || '%'
  OR university ILIKE '%' || $8 || '%')
 AND (NULLIF($9::text, '') IS NULL
  OR COALESCE(funding->>'tuition', 'false') = NULLIF($9::text, ''))
ORDER BY deadline ASC NULLS LAST, confidence_overall DESC
LIMIT $10 OFFSET $11;

-- name: ListScholarshipsBySpace :many
SELECT * FROM scholarships WHERE country_space_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListScholarshipsByCountry :many
SELECT * FROM scholarships WHERE country = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountScholarshipsByCountry :many
SELECT country, count(*) as count FROM scholarships GROUP BY country;

-- name: DeleteScholarship :exec
DELETE FROM scholarships WHERE id = $1;
