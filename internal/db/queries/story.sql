-- Story meta sync queries (§79 Community Story Wall).
-- The contents service keeps story_meta in sync for content_type='story'
-- rows created or updated through the generic contents pipeline (/create
-- composer, AI tools). The dedicated /stories endpoints in internal/story
-- write the same table directly via pgx.

-- name: GetCountrySpaceIDByGeoCode :one
SELECT id
FROM spaces
WHERE level = 'country'
 AND geo_code = @geo_code
 AND status = 'active'
 AND deleted_at IS NULL;

-- name: GetStoryMetaByContent :one
-- Full row so the caller can merge per-field (field_values keys absent from
-- an update keep their existing values instead of being wiped to NULL).
SELECT id, content_id, origin_space_id, destination_space_id,
 relocation_year, visa_category, profession, biggest_challenge,
 most_useful_resource, best_advice, verified,
 ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
 hitl_reviewed_at, is_japabot_indexed, created_at, updated_at
FROM story_meta
WHERE content_id = @content_id;

-- name: InsertStoryMeta :exec
-- Upsert on the (content_id) unique key — the composer sync and
-- the dedicated /stories service both write this table; a story created
-- through one path and completed through the other must not produce a
-- duplicate row (migration 000620).
INSERT INTO story_meta (content_id, origin_space_id, destination_space_id, relocation_year, visa_category, profession, biggest_challenge, most_useful_resource, best_advice) VALUES (@content_id, sqlc.narg(origin_space_id), @destination_space_id, sqlc.narg(relocation_year), sqlc.narg(visa_category), sqlc.narg(profession), sqlc.narg(biggest_challenge), sqlc.narg(most_useful_resource), sqlc.narg(best_advice))

ON CONFLICT (content_id) DO UPDATE SET
 origin_space_id = COALESCE(EXCLUDED.origin_space_id, story_meta.origin_space_id),
 destination_space_id = EXCLUDED.destination_space_id,
 relocation_year = COALESCE(EXCLUDED.relocation_year, story_meta.relocation_year),
 visa_category = COALESCE(EXCLUDED.visa_category, story_meta.visa_category),
 profession = COALESCE(EXCLUDED.profession, story_meta.profession),
 biggest_challenge = COALESCE(EXCLUDED.biggest_challenge, story_meta.biggest_challenge),
 most_useful_resource = COALESCE(EXCLUDED.most_useful_resource, story_meta.most_useful_resource),
 best_advice = COALESCE(EXCLUDED.best_advice, story_meta.best_advice),
 updated_at = now();

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
WHERE content_id = @content_id;
