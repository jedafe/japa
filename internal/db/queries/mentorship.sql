-- name: ListMentorshipPrograms :many
SELECT id, name, description, is_active, created_at, updated_at
FROM mentorship_programs

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMentorshipProgram :one
SELECT id, name, description, is_active, created_at, updated_at
FROM mentorship_programs
WHERE id = @id;

-- name: CreateMentorshipProgram :one
INSERT INTO mentorship_programs (name, description, is_active) VALUES (@name, @description, @is_active)

RETURNING id, name, description, is_active, created_at, updated_at;

-- name: UpdateMentorshipProgram :one
UPDATE mentorship_programs
SET name = @name, description = @description, is_active = @is_active, updated_at = now()
WHERE id = @id
RETURNING id, name, description, is_active, created_at, updated_at;

-- name: ListMentorshipMatches :many
SELECT m.id, m.program_id, m.mentor_id, m.mentee_id, m.status, m.created_at, m.updated_at,
 p.name AS program_name,
 COALESCE(mu.public_username, mu.username, '') AS mentor_username,
 COALESCE(eu.public_username, eu.username, '') AS mentee_username
FROM mentorship_matches m
LEFT JOIN mentorship_programs p ON p.id = m.program_id
LEFT JOIN users mu ON mu.id = m.mentor_id
LEFT JOIN users eu ON eu.id = m.mentee_id

ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateMentorshipMatch :one
UPDATE mentorship_matches
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING id, program_id, mentor_id, mentee_id, status, created_at, updated_at;

-- name: DeleteMentorshipProgram :execrows
DELETE FROM mentorship_programs
WHERE id = @id;

-- name: ListMentorshipProgramsWithCounts :many
SELECT p.id, p.name, p.description, p.is_active, p.created_at, p.updated_at,
 COALESCE(mentor_counts.cnt, 0)::int AS mentor_count,
 COALESCE(mentee_counts.cnt, 0)::int AS mentee_count,
 COALESCE(match_counts.cnt, 0)::int AS match_count
FROM mentorship_programs p
LEFT JOIN (
 SELECT mm.program_id, COUNT(DISTINCT mm.mentor_id) AS cnt
 FROM mentorship_matches mm
 GROUP BY mm.program_id
) mentor_counts ON mentor_counts.program_id = p.id
LEFT JOIN (
 SELECT mm.program_id, COUNT(DISTINCT mm.mentee_id) AS cnt
 FROM mentorship_matches mm
 GROUP BY mm.program_id
) mentee_counts ON mentee_counts.program_id = p.id
LEFT JOIN (
 SELECT mm.program_id, COUNT(*) AS cnt
 FROM mentorship_matches mm
 GROUP BY mm.program_id
) match_counts ON match_counts.program_id = p.id

ORDER BY p.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- §102 Mentorship Matching queries (ADR-110)
-- Ref: japaconnect-engineering/sections/102-mentorship-matching.md
-- =====================================================================

-- name: GetMentorProfile :one
-- Get the current user's mentor profile settings from user_profiles.
-- username and mentor_listing_content_id join in for the shadow-listing
-- upsert in UpdateMentorProfile (000824 unification).
SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, up.mentor_listing_content_id, COALESCE(u.public_username, u.username, '') AS username,
 up.profession, up.website_url,
 -- 000829 mentor card fields (§114 detail-page rebuild).
 up.location, up.mentor_title, up.mentor_response_time, up.mentor_availability,
 up.mentor_session_type, up.mentor_languages, up.mentor_session_formats,
 up.mentor_countries, up.mentor_credentials, up.mentor_years_experience,
 up.mentor_success_label
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.user_id = @user_id;

-- name: UpdateMentorProfile :one
-- Toggle is_mentor and set capacity/areas/bio. When is_mentor flips to true,
-- mentor_since is set to now() if not already recorded. mentor_since is
-- deliberately NOT cleared when is_mentor turns off — it records when the
-- user first mentored and is restored intact on re-enable.
UPDATE user_profiles
SET is_mentor = @is_mentor,
 mentor_capacity = @mentor_capacity,
 mentor_areas = @mentor_areas,
 mentor_bio = @mentor_bio,
 -- 000829 mentor card fields — COALESCE so nil preserves the stored
 -- value; the service layer maps absent → nil, empty → empty value.
 mentor_title = COALESCE(sqlc.narg(mentor_title), mentor_title),
 mentor_response_time = COALESCE(sqlc.narg(mentor_response_time), mentor_response_time),
 mentor_availability = COALESCE(sqlc.narg(mentor_availability), mentor_availability),
 mentor_session_type = COALESCE(sqlc.narg(mentor_session_type), mentor_session_type),
 mentor_languages = COALESCE(sqlc.narg(mentor_languages), mentor_languages),
 mentor_session_formats = COALESCE(sqlc.narg(mentor_session_formats), mentor_session_formats),
 mentor_countries = COALESCE(sqlc.narg(mentor_countries), mentor_countries),
 mentor_credentials = COALESCE(sqlc.narg(mentor_credentials), mentor_credentials),
 mentor_years_experience = COALESCE(sqlc.narg(mentor_years_experience), mentor_years_experience),
 mentor_success_label = COALESCE(sqlc.narg(mentor_success_label), mentor_success_label),
 mentor_since = CASE WHEN @is_mentor = true AND mentor_since IS NULL THEN now() ELSE mentor_since END,
 updated_at = now()
WHERE user_id = @user_id
RETURNING user_id, is_mentor, mentor_capacity, active_mentee_count,
 mentor_areas, mentor_bio, mentor_since, display_name, avatar_url,
 profession, website_url,
 mentor_title, mentor_response_time, mentor_availability,
 mentor_session_type, mentor_languages, mentor_session_formats,
 mentor_countries, mentor_credentials, mentor_years_experience,
 mentor_success_label;

-- name: GetMentorByUsername :one
-- Get a mentor's public profile by username (is_mentor must be true).
-- 000824: the mentor must also have a live shadow listing —
-- listing_kind='mentorship' meta on a published, non-deleted contents row.
SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, COALESCE(u.public_username, u.username, '') AS username,
 up.profession, up.website_url
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE (u.username = @username OR lower(u.public_username) = lower(@username)) AND up.is_mentor = true
 AND EXISTS (
   SELECT 1 FROM contents sc
   JOIN service_listing_meta sm ON sm.content_id = sc.id
   WHERE sc.author_id = up.user_id
     AND sc.content_type = 'service_listing'
     AND sm.listing_kind = 'mentorship'
     AND sc.status = 'published'
     AND sc.deleted_at IS NULL
 );

-- name: ListMentors :many
-- Discover mentors with optional destination and area filters.
-- dest: when provided, filters mentors whose mentor_areas mention the
-- destination country name, geo_code, slug, or any word of the name.
-- sort: 'newest' (mentor_since DESC), 'capacity' (available_slots DESC),
-- 'match' (keyword area_overlap DESC — partial substring match).
-- include_unavailable: when false (default) at-capacity mentors
-- (available_slots <= 0) are hidden from discovery
-- (§102/§111 hardening amendment).
WITH mentors AS (
 SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, COALESCE(u.public_username, u.username, '') AS username, up.profession, up.website_url,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 COUNT(*) OVER() AS total_count,
 COALESCE(
 (SELECT COUNT(*)::double precision / NULLIF(array_length(@areas::text[], 1), 0)
 FROM unnest(@areas::text[]) AS a
 WHERE length(a) >= 2
 AND EXISTS (
 SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || regexp_replace(a, '([\\%_])', '\\\1', 'g') || '%' ESCAPE '\'
 )),
 0
 ) AS area_overlap
 FROM user_profiles up
 JOIN users u ON u.id = up.user_id
 WHERE up.is_mentor = true
 -- 000824: only mentors with a live shadow listing are discoverable.
 AND EXISTS (
   SELECT 1 FROM contents sc
   JOIN service_listing_meta sm ON sm.content_id = sc.id
   WHERE sc.author_id = up.user_id
     AND sc.content_type = 'service_listing'
     AND sm.listing_kind = 'mentorship'
     AND sc.status = 'published'
     AND sc.deleted_at IS NULL
 )
 AND (
 @dest_space_id::uuid IS NULL OR EXISTS (
 SELECT 1 FROM spaces s
 WHERE s.id = @dest_space_id
 AND (
 array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.name || '%'
 OR (s.geo_code IS NOT NULL AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.geo_code || '%')
 OR (s.slug IS NOT NULL AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.slug || '%')
 OR EXISTS (
 SELECT 1 FROM unnest(string_to_array(s.name, ' ')) AS word
 WHERE word <> '' AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || word || '%'
 )
 OR array_to_string(up.mentor_areas, ' ') ILIKE '%' || (
 SELECT upper(string_agg(left(regexp_replace(word, '[^a-zA-Z]', '', 'g'), 1), ''))
 FROM unnest(string_to_array(regexp_replace(s.name, '\y(the|of|and|in)\y', '', 'gi'), ' ')) AS word
 WHERE regexp_replace(word, '[^a-zA-Z]', '', 'g') <> ''
 ) || '%'
 )
 )
 )
 AND (
 @areas::text[] IS NULL
 OR COALESCE(array_length(@areas::text[], 1), 0) = 0
 OR EXISTS (
 SELECT 1 FROM unnest(@areas::text[]) AS a
 WHERE length(a) >= 2
 AND EXISTS (
 SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || regexp_replace(a, '([\\%_])', '\\\1', 'g') || '%' ESCAPE '\'
 )
 )
 )
 AND (@include_unavailable::boolean OR up.mentor_capacity > up.active_mentee_count)
)
SELECT user_id, is_mentor, mentor_capacity,
 active_mentee_count, mentor_areas, mentor_bio, mentor_since,
 display_name, avatar_url, username, profession, website_url,
 available_slots, total_count, area_overlap
FROM mentors
ORDER BY
 CASE WHEN @sort = 'match' THEN area_overlap END DESC,
 CASE WHEN @sort = 'capacity' THEN available_slots END DESC,
 mentor_since DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: ListMentorsByMatch :many
-- Discover mentors ordered by §102 match score (60% keyword + 40% bio cosine).
-- Mentors without an embedding fall back to keyword-only scoring (weight=1.0).
-- include_unavailable: same at-capacity semantics as ListMentors.
WITH mentors AS (
 SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, COALESCE(u.public_username, u.username, '') AS username, up.profession, up.website_url,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 COUNT(*) OVER() AS total_count,
 CASE WHEN up.mentor_embedding IS NOT NULL
 THEN (1 - (up.mentor_embedding <=> @mentee_embedding))::double precision
 ELSE 0::double precision
 END AS bio_similarity,
 COALESCE(
 (SELECT COUNT(*)::double precision / NULLIF(array_length(@areas::text[], 1), 0)
 FROM unnest(@areas::text[]) AS a
 WHERE length(a) >= 2
 AND EXISTS (
 SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || regexp_replace(a, '([\\%_])', '\\\1', 'g') || '%' ESCAPE '\'
 )),
 0
 ) AS area_overlap
 FROM user_profiles up
 JOIN users u ON u.id = up.user_id
 WHERE up.is_mentor = true
 -- 000824: only mentors with a live shadow listing are discoverable.
 AND EXISTS (
   SELECT 1 FROM contents sc
   JOIN service_listing_meta sm ON sm.content_id = sc.id
   WHERE sc.author_id = up.user_id
     AND sc.content_type = 'service_listing'
     AND sm.listing_kind = 'mentorship'
     AND sc.status = 'published'
     AND sc.deleted_at IS NULL
 )
 AND (
 @dest_space_id::uuid IS NULL OR EXISTS (
 SELECT 1 FROM spaces s
 WHERE s.id = @dest_space_id
 AND (
 array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.name || '%'
 OR (s.geo_code IS NOT NULL AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.geo_code || '%')
 OR (s.slug IS NOT NULL AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || s.slug || '%')
 OR EXISTS (
 SELECT 1 FROM unnest(string_to_array(s.name, ' ')) AS word
 WHERE word <> '' AND array_to_string(up.mentor_areas, ' ') ILIKE '%' || word || '%'
 )
 OR array_to_string(up.mentor_areas, ' ') ILIKE '%' || (
 SELECT upper(string_agg(left(regexp_replace(word, '[^a-zA-Z]', '', 'g'), 1), ''))
 FROM unnest(string_to_array(regexp_replace(s.name, '\y(the|of|and|in)\y', '', 'gi'), ' ')) AS word
 WHERE regexp_replace(word, '[^a-zA-Z]', '', 'g') <> ''
 ) || '%'
 )
 )
 )
 AND (
 @areas::text[] IS NULL
 OR COALESCE(array_length(@areas::text[], 1), 0) = 0
 OR EXISTS (
 SELECT 1 FROM unnest(@areas::text[]) AS a
 WHERE length(a) >= 2
 AND EXISTS (
 SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || regexp_replace(a, '([\\%_])', '\\\1', 'g') || '%' ESCAPE '\'
 )
 )
 )
 AND (@include_unavailable::boolean OR up.mentor_capacity > up.active_mentee_count)
)
SELECT user_id, is_mentor, mentor_capacity,
 active_mentee_count, mentor_areas, mentor_bio, mentor_since,
 display_name, avatar_url, username, profession, website_url,
 available_slots, total_count, bio_similarity, area_overlap,
 (0.6 * area_overlap + 0.4 * bio_similarity) AS match_score
FROM mentors
ORDER BY match_score DESC, mentor_since DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMentorsByIDs :many
-- Get up to 4 mentors by user IDs for the comparison endpoint (§30).
-- 000824: requires a live shadow listing — an is_mentor profile without a
-- published mentorship listing is not comparable.
SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, COALESCE(u.public_username, u.username, '') AS username, up.profession, up.website_url,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.is_mentor = true
 AND up.user_id = ANY(@user_ids::uuid[])
 AND EXISTS (
   SELECT 1 FROM contents sc
   JOIN service_listing_meta sm ON sm.content_id = sc.id
   WHERE sc.author_id = up.user_id
     AND sc.content_type = 'service_listing'
     AND sm.listing_kind = 'mentorship'
     AND sc.status = 'published'
     AND sc.deleted_at IS NULL
 )
ORDER BY u.username;

-- name: SearchMentors :many
-- Username/display-name substring search over discoverable mentors —
-- same 000824 live-shadow gate as the other discovery queries.
SELECT up.user_id, up.is_mentor, up.mentor_capacity,
 up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
 up.display_name, up.avatar_url, COALESCE(u.public_username, u.username, '') AS username, up.profession, up.website_url,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.is_mentor = true
 AND (u.username ILIKE @q OR up.display_name ILIKE @q)
 AND EXISTS (
   SELECT 1 FROM contents sc
   JOIN service_listing_meta sm ON sm.content_id = sc.id
   WHERE sc.author_id = up.user_id
     AND sc.content_type = 'service_listing'
     AND sm.listing_kind = 'mentorship'
     AND sc.status = 'published'
     AND sc.deleted_at IS NULL
 )
ORDER BY up.display_name, u.username
LIMIT @page_limit;

-- name: SetMentorListingContentID :exec
-- Link/unlink the mentor's shadow listing on user_profiles (000824).
-- NULL clears the link (used when the mentor opts out and the shadow
-- contents row is soft-deleted).
UPDATE user_profiles
SET mentor_listing_content_id = sqlc.narg(content_id), updated_at = now()
WHERE user_id = @user_id;


-- name: UpdateMentorEmbedding :exec
-- Set the pgvector embedding for a mentor's bio (called by Asynq task).
UPDATE user_profiles
SET mentor_embedding = @embedding, updated_at = now()
WHERE user_id = @user_id;

-- name: GetMentorEmbedding :one
-- Get a mentor's embedding vector (for match scoring).
SELECT user_id, mentor_embedding
FROM user_profiles
WHERE user_id = @user_id AND is_mentor = true;

-- name: GetUserEmbedding :one
-- Get any user's mentor_embedding vector (for match scoring on the discovery
-- page). Unlike GetMentorEmbedding, this does NOT require is_mentor = true —
-- a mentee who is also a mentor will have an embedding from their own bio.
-- Returns NULL for users without an embedding (service falls back to
-- keyword-only scoring, §102).
SELECT user_id, mentor_embedding
FROM user_profiles
WHERE user_id = @user_id;

-- name: GetMenteeDestinationSpaceID :one
-- Get the authenticated user's destination space ID (japa_stage_dest_space_id,
-- §87) for use as the default ?dest= filter on the mentor discovery page
-- (§102 Geo Awareness: "The mentee's profile destination_country_id is
-- injected as a default filter when logged in, with no UI action required").
SELECT user_id, japa_stage_dest_space_id AS destination_space_id
FROM user_profiles
WHERE user_id = @user_id;

-- name: ListMentorsWithEmbeddings :many
-- Get all mentors with their embeddings for batch Meilisearch sync.
-- Same live-shadow EXISTS gate as the sibling discovery queries (000824):
-- a stale is_mentor flag without a live published shadow listing is not
-- an indexable mentor document.
SELECT up.user_id, up.mentor_embedding, up.is_mentor, up.mentor_areas,
 up.mentor_bio, up.mentor_capacity, up.active_mentee_count, up.mentor_since
FROM user_profiles up
WHERE up.is_mentor = true
 AND EXISTS (
     SELECT 1 FROM contents c
     JOIN service_listing_meta sm ON sm.content_id = c.id
     WHERE c.author_id = up.user_id
       AND c.content_type = 'service_listing'
       AND sm.listing_kind = 'mentorship'
       AND c.status = 'published'
       AND c.deleted_at IS NULL
 );
