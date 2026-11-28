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
 u.verified, u.verification_type,
 up.profession, up.website_url,
 -- 000829 mentor card fields (§114 detail-page rebuild).
 up.location, up.mentor_title, up.mentor_response_time, up.mentor_availability,
 up.mentor_session_type, up.mentor_languages, up.mentor_session_formats,
 up.mentor_countries, up.mentor_credentials, up.mentor_years_experience,
 up.mentor_success_label
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.user_id = @user_id;

-- name: GetMentorProfileUserFallback :one
-- users-only row for accounts WITHOUT a user_profiles row (migrated or
-- anomalous accounts). Lets /me/mentor-profile resolve the aliased
-- username + verification flags instead of 500ing; the UpdateMentorProfile
-- upsert creates the profile row lazily on first save.
SELECT u.id AS user_id,
 COALESCE(u.public_username, u.username, '') AS username,
 u.verified, u.verification_type
FROM users u
WHERE u.id = @user_id;

-- name: UpdateMentorProfile :one
-- Toggle is_mentor and set capacity/areas/bio — upsert so a missing
-- user_profiles row is created rather than erroring (audit GAP-01).
-- When is_mentor flips to true, mentor_since is set to now() if not
-- already recorded. mentor_since is deliberately NOT cleared when
-- is_mentor turns off — it records when the user first mentored and is
-- restored intact on re-enable.
INSERT INTO user_profiles (user_id, is_mentor, mentor_capacity, mentor_areas, mentor_bio,
 mentor_title, mentor_response_time, mentor_availability, mentor_session_type,
 mentor_languages, mentor_session_formats, mentor_countries, mentor_credentials,
 mentor_years_experience, mentor_success_label, mentor_since)
VALUES (@user_id, @is_mentor, @mentor_capacity, @mentor_areas, @mentor_bio,
 sqlc.narg(mentor_title), sqlc.narg(mentor_response_time), sqlc.narg(mentor_availability), sqlc.narg(mentor_session_type),
 sqlc.narg(mentor_languages), sqlc.narg(mentor_session_formats), sqlc.narg(mentor_countries), sqlc.narg(mentor_credentials),
 sqlc.narg(mentor_years_experience), sqlc.narg(mentor_success_label),
 CASE WHEN @is_mentor = true THEN now() ELSE NULL END)
ON CONFLICT (user_id) DO UPDATE
SET is_mentor = EXCLUDED.is_mentor,
 mentor_capacity = EXCLUDED.mentor_capacity,
 mentor_areas = EXCLUDED.mentor_areas,
 mentor_bio = EXCLUDED.mentor_bio,
 -- 000829 mentor card fields — COALESCE(EXCLUDED) so nil preserves the
 -- stored value; the service layer maps absent → nil, empty → empty value.
 mentor_title = COALESCE(EXCLUDED.mentor_title, user_profiles.mentor_title),
 mentor_response_time = COALESCE(EXCLUDED.mentor_response_time, user_profiles.mentor_response_time),
 mentor_availability = COALESCE(EXCLUDED.mentor_availability, user_profiles.mentor_availability),
 mentor_session_type = COALESCE(EXCLUDED.mentor_session_type, user_profiles.mentor_session_type),
 mentor_languages = COALESCE(EXCLUDED.mentor_languages, user_profiles.mentor_languages),
 mentor_session_formats = COALESCE(EXCLUDED.mentor_session_formats, user_profiles.mentor_session_formats),
 mentor_countries = COALESCE(EXCLUDED.mentor_countries, user_profiles.mentor_countries),
 mentor_credentials = COALESCE(EXCLUDED.mentor_credentials, user_profiles.mentor_credentials),
 mentor_years_experience = COALESCE(EXCLUDED.mentor_years_experience, user_profiles.mentor_years_experience),
 mentor_success_label = COALESCE(EXCLUDED.mentor_success_label, user_profiles.mentor_success_label),
 mentor_since = CASE WHEN EXCLUDED.is_mentor = true AND user_profiles.mentor_since IS NULL THEN now() ELSE user_profiles.mentor_since END,
 updated_at = now()
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
 u.verified, u.verification_type,
 up.profession, up.website_url,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 -- 000940: mentor card fields (000829) + location + mentees-helped count +
 -- shadow-listing package price — power the mentors comparison dimensions.
 up.location, up.mentor_title, up.mentor_response_time,
 up.mentor_availability, up.mentor_session_type,
 up.mentor_languages, up.mentor_session_formats,
 up.mentor_countries, up.mentor_credentials,
 up.mentor_years_experience, up.mentor_success_label,
 (SELECT COUNT(*)::int FROM service_bookings sb
  WHERE sb.provider_user_id = up.user_id AND sb.status = 'fulfilled') AS mentees_helped,
 ml.price_amount AS package_price, ml.price_currency AS package_currency, ml.price_per AS package_per
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN LATERAL (
 SELECT sm.price_amount, sm.price_currency, sm.price_per
 FROM contents sc
 JOIN service_listing_meta sm ON sm.content_id = sc.id
 WHERE sc.author_id = up.user_id
   AND sc.content_type = 'service_listing'
   AND sm.listing_kind = 'mentorship'
   AND sc.status = 'published'
   AND sc.deleted_at IS NULL
 ORDER BY sc.id
 LIMIT 1
) ml ON true
WHERE (u.username = @username OR lower(u.public_username) = lower(@username)
 -- §50 former-username amendment: resolves a former handle to the owner's
 -- current canonical; inert while any live handle claims the name.
 OR u.username = (SELECT u2.username FROM username_redirects ur
                  JOIN users u2 ON u2.id = ur.user_id AND u2.status != 'deleted'
                 WHERE ur.username = @username
                   AND NOT EXISTS (SELECT 1 FROM users ux
                                    WHERE ux.username = @username
                                       OR lower(ux.public_username) = lower(@username))))
 AND up.is_mentor = true AND u.status <> 'banned' AND EXISTS (SELECT 1 FROM contents pc WHERE pc.id = up.mentor_listing_content_id AND pc.status = 'published' AND pc.deleted_at IS NULL)
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
 u.verified, u.verification_type,
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
 WHERE up.is_mentor = true AND u.status <> 'banned' AND EXISTS (SELECT 1 FROM contents pc WHERE pc.id = up.mentor_listing_content_id AND pc.status = 'published' AND pc.deleted_at IS NULL)
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
 verified, verification_type,
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
 u.verified, u.verification_type,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 COUNT(*) OVER() AS total_count,
 CASE WHEN up.mentor_embedding IS NOT NULL
 THEN (1 - (up.mentor_embedding <=> @mentee_embedding))::double precision
 ELSE NULL::double precision
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
 WHERE up.is_mentor = true AND u.status <> 'banned' AND EXISTS (SELECT 1 FROM contents pc WHERE pc.id = up.mentor_listing_content_id AND pc.status = 'published' AND pc.deleted_at IS NULL)
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
 verified, verification_type,
 available_slots, total_count, bio_similarity, area_overlap,
 -- §102 fallback: a mentor without an embedding scores keyword-only at
 -- weight 1.0 (not 0.6) so they aren't penalised while their embedding
 -- job is still pending.
 CASE WHEN bio_similarity IS NULL THEN area_overlap
      ELSE (0.6 * area_overlap + 0.4 * bio_similarity) END AS match_score
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
 u.verified, u.verification_type,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 -- 000940: mentor card fields (000829) + location + mentees-helped count +
 -- shadow-listing package price — power the mentors comparison dimensions.
 up.location, up.mentor_title, up.mentor_response_time,
 up.mentor_availability, up.mentor_session_type,
 up.mentor_languages, up.mentor_session_formats,
 up.mentor_countries, up.mentor_credentials,
 up.mentor_years_experience, up.mentor_success_label,
 (SELECT COUNT(*)::int FROM service_bookings sb
  WHERE sb.provider_user_id = up.user_id AND sb.status = 'fulfilled') AS mentees_helped,
 ml.price_amount AS package_price, ml.price_currency AS package_currency, ml.price_per AS package_per
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN LATERAL (
 SELECT sm.price_amount, sm.price_currency, sm.price_per
 FROM contents sc
 JOIN service_listing_meta sm ON sm.content_id = sc.id
 WHERE sc.author_id = up.user_id
   AND sc.content_type = 'service_listing'
   AND sm.listing_kind = 'mentorship'
   AND sc.status = 'published'
   AND sc.deleted_at IS NULL
 ORDER BY sc.id
 LIMIT 1
) ml ON true
WHERE up.is_mentor = true AND u.status <> 'banned' AND EXISTS (SELECT 1 FROM contents pc WHERE pc.id = up.mentor_listing_content_id AND pc.status = 'published' AND pc.deleted_at IS NULL)
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
 u.verified, u.verification_type,
 (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
 -- 000940: mentor card fields (000829) + location + mentees-helped count +
 -- shadow-listing package price — power the mentors comparison dimensions.
 up.location, up.mentor_title, up.mentor_response_time,
 up.mentor_availability, up.mentor_session_type,
 up.mentor_languages, up.mentor_session_formats,
 up.mentor_countries, up.mentor_credentials,
 up.mentor_years_experience, up.mentor_success_label,
 (SELECT COUNT(*)::int FROM service_bookings sb
  WHERE sb.provider_user_id = up.user_id AND sb.status = 'fulfilled') AS mentees_helped,
 ml.price_amount AS package_price, ml.price_currency AS package_currency, ml.price_per AS package_per
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN LATERAL (
 SELECT sm.price_amount, sm.price_currency, sm.price_per
 FROM contents sc
 JOIN service_listing_meta sm ON sm.content_id = sc.id
 WHERE sc.author_id = up.user_id
   AND sc.content_type = 'service_listing'
   AND sm.listing_kind = 'mentorship'
   AND sc.status = 'published'
   AND sc.deleted_at IS NULL
 ORDER BY sc.id
 LIMIT 1
) ml ON true
WHERE up.is_mentor = true AND u.status <> 'banned' AND EXISTS (SELECT 1 FROM contents pc WHERE pc.id = up.mentor_listing_content_id AND pc.status = 'published' AND pc.deleted_at IS NULL)
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
SELECT user_id, COALESCE(mentee_goal_embedding, mentor_embedding) AS mentor_embedding
FROM user_profiles
WHERE user_id = @user_id;

-- name: GetLatestMenteeGoals :one
-- The mentee-side match source (§102 mentee_goal_embedding): the most
-- recent goals text the user submitted with a mentorship request — meta's
-- 'goals' wins, the note column is the fallback for migrated rows.
SELECT COALESCE(sb.meta->>'goals', sb.note, '') AS goals
FROM service_bookings sb
JOIN service_listing_meta sm ON sm.id = sb.listing_meta_id
WHERE sm.listing_kind = 'mentorship'
  AND sb.booker_user_id = @user_id
  AND COALESCE(sb.meta->>'goals', sb.note, '') <> ''
ORDER BY sb.created_at DESC
LIMIT 1;

-- name: UpdateMenteeGoalEmbedding :exec
UPDATE user_profiles SET mentee_goal_embedding = @mentee_goal_embedding, updated_at = now()
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

-- =====================================================================
-- §102 2026-11-28 amendment — member mentor applications
-- =====================================================================

-- name: CreateMentorApplication :one
-- First submission — one live application row per member (UNIQUE user_id).
-- Resubmission after a decline goes through ResubmitMentorApplication.
INSERT INTO mentor_applications (user_id, motivation, mentor_areas)
VALUES (@user_id, @motivation, @mentor_areas)
RETURNING *;

-- name: ResubmitMentorApplication :one
-- Re-apply after a decline (or an approved-but-orphaned row whose group
-- grant was removed out-of-band): back to pending, reviewer fields and
-- decline_reason cleared, fresh motivation/areas.
UPDATE mentor_applications
SET motivation = @motivation,
    mentor_areas = @mentor_areas,
    status = 'pending',
    reviewer_user_id = NULL,
    reviewed_at = NULL,
    decline_reason = NULL,
    updated_at = now()
WHERE user_id = @user_id
  AND status IN ('declined','approved')
RETURNING *;

-- name: GetMentorApplicationByUser :one
SELECT * FROM mentor_applications
WHERE user_id = @user_id;

-- name: GetMentorApplicationByID :one
SELECT * FROM mentor_applications
WHERE id = @id;

-- name: ListMentorApplications :many
-- Admin review queue (§102 2026-11-28). Pending-first ordering keeps the
-- actionable rows on top; ?status= narrows to one bucket, ?q= searches
-- username/public username/display name.
SELECT a.*, COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name
FROM mentor_applications a
JOIN users u ON u.id = a.user_id
LEFT JOIN user_profiles up ON up.user_id = a.user_id
WHERE (@status::text = '' OR a.status = @status)
  AND (@q::text = ''
       OR u.username ILIKE '%' || @q || '%'
       OR u.public_username ILIKE '%' || @q || '%'
       OR up.display_name ILIKE '%' || @q || '%')
ORDER BY CASE WHEN a.status = 'pending' THEN 0 ELSE 1 END, a.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMentorApplications :one
SELECT COUNT(*)
FROM mentor_applications a
JOIN users u ON u.id = a.user_id
LEFT JOIN user_profiles up ON up.user_id = a.user_id
WHERE (@status::text = '' OR a.status = @status)
  AND (@q::text = ''
       OR u.username ILIKE '%' || @q || '%'
       OR u.public_username ILIKE '%' || @q || '%'
       OR up.display_name ILIKE '%' || @q || '%');

-- name: ApproveMentorApplication :one
-- pending → approved; records the reviewer + timestamp. The callers adds
-- the applicant to the mentors group in the same service call.
UPDATE mentor_applications
SET status = 'approved',
    reviewer_user_id = @reviewer_user_id,
    reviewed_at = now(),
    decline_reason = NULL,
    updated_at = now()
WHERE id = @id
  AND status = 'pending'
RETURNING *;

-- name: DeclineMentorApplication :one
-- pending → declined (reject) or approved → declined (revoke — the
-- caller removes the mentors membership, firing the mentor-retire sweep).
UPDATE mentor_applications
SET status = 'declined',
    reviewer_user_id = @reviewer_user_id,
    reviewed_at = now(),
    decline_reason = @decline_reason,
    updated_at = now()
WHERE id = @id
  AND status IN ('pending','approved')
RETURNING *;
