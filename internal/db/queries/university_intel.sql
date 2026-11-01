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
-- Merge field keys into the override set (admin-edit marking).
UPDATE university_intel_links
SET editorial_overrides = editorial_overrides || @overrides::jsonb,
    updated_at = now()
WHERE space_id = @space_id;

-- name: GetUniversitySpaceBySlug :one
-- University = spaces row on the 'education' space type with level='university'
-- (migration 000970 retired the dedicated university type).
SELECT s.id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
 s.wiki_content, s.logo_media_id, s.geo_code, s.level, s.visibility,
 s.posting_perm, s.replying_perm, s.commenting_perm, s.joining_perm, s.status
FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE st.slug = 'education' AND s.level = 'university' AND s.slug = @slug
  AND s.deleted_at IS NULL
LIMIT 1;

-- name: GetUniversitySpaceByName :many
-- Name-based university candidates for a course-offering university_name that
-- did not resolve by slug — the promoter narrows these by country match.
-- Exact match first, then containment (extracted text often carries the
-- "…(MIT)" suffix the space name lacks), then pg_trgm similarity for
-- spelling variants; genuinely ambiguous strings like bare "MIT" still
-- produce multiple candidates and route to review.
SELECT s.id, s.name, s.slug, s.geo_code
FROM spaces s
JOIN space_types st ON st.id = s.space_type_id
WHERE st.slug = 'education' AND s.level = 'university'
  AND s.deleted_at IS NULL
  AND (lower(s.name) = lower(@name)
       OR position(lower(s.name) in lower(@name)) > 0
       OR similarity(s.name, @name) > 0.4)
ORDER BY (lower(s.name) = lower(@name)) DESC, similarity(s.name, @name) DESC
LIMIT 5;

-- name: RemoveUniversityIntelOverrides :exec
-- Releases field keys from a university's editorial-overrides set — the
-- admin "re-crawl may write this field again" path. jsonb '-' removes the
-- listed keys in place; other keys are untouched.
UPDATE university_intel_links
SET editorial_overrides = editorial_overrides - $2::text[]
WHERE space_id = $1;
