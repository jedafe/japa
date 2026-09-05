-- Story meta sync queries (§79 Community Story Wall).
-- The contents service keeps story_meta in sync for content_type='story'
-- rows created or updated through the generic contents pipeline (/create
-- composer, AI tools). The dedicated /stories endpoints in internal/story
-- write the same table directly via pgx.

-- name: GetCountrySpaceIDByGeoCode :one
SELECT id
FROM spaces
WHERE tenant_id = @tenant_id
  AND level = 'country'
  AND geo_code = @geo_code
  AND status = 'active'
  AND deleted_at IS NULL;

-- name: GetStoryMetaByContent :one
SELECT id
FROM story_meta
WHERE content_id = @content_id AND tenant_id = @tenant_id
LIMIT 1;

-- name: InsertStoryMeta :exec
INSERT INTO story_meta (tenant_id, content_id, origin_space_id, destination_space_id,
    relocation_year, visa_category, profession, biggest_challenge,
    most_useful_resource, best_advice)
VALUES (@tenant_id, @content_id, sqlc.narg(origin_space_id), @destination_space_id,
    sqlc.narg(relocation_year), sqlc.narg(visa_category), sqlc.narg(profession),
    sqlc.narg(biggest_challenge), sqlc.narg(most_useful_resource), sqlc.narg(best_advice));

-- name: UpdateStoryMetaByContent :exec
UPDATE story_meta
SET origin_space_id = sqlc.narg(origin_space_id),
    destination_space_id = @destination_space_id,
    relocation_year = sqlc.narg(relocation_year),
    visa_category = sqlc.narg(visa_category),
    profession = sqlc.narg(profession),
    biggest_challenge = sqlc.narg(biggest_challenge),
    most_useful_resource = sqlc.narg(most_useful_resource),
    best_advice = sqlc.narg(best_advice),
    updated_at = now()
WHERE content_id = @content_id AND tenant_id = @tenant_id;
