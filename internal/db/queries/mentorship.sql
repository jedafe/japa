-- Mentorship program and match queries (DB-001: migrated from raw SQL to sqlc).

-- name: ListMentorshipPrograms :many
SELECT id, tenant_id, name, description, is_active, created_at, updated_at
FROM mentorship_programs
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMentorshipProgram :one
SELECT id, tenant_id, name, description, is_active, created_at, updated_at
FROM mentorship_programs
WHERE tenant_id = @tenant_id AND id = @id;

-- name: CreateMentorshipProgram :one
INSERT INTO mentorship_programs (tenant_id, name, description, is_active)
VALUES (@tenant_id, @name, @description, @is_active)
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: UpdateMentorshipProgram :one
UPDATE mentorship_programs
SET name = @name, description = @description, is_active = @is_active, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, name, description, is_active, created_at, updated_at;

-- name: ListMentorshipMatches :many
SELECT m.id, m.tenant_id, m.program_id, m.mentor_id, m.mentee_id, m.status, m.created_at, m.updated_at,
       p.name AS program_name,
       mu.username AS mentor_username,
       eu.username AS mentee_username
FROM mentorship_matches m
LEFT JOIN mentorship_programs p ON p.id = m.program_id
LEFT JOIN users mu ON mu.id = m.mentor_id
LEFT JOIN users eu ON eu.id = m.mentee_id
WHERE m.tenant_id = @tenant_id
ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateMentorshipMatch :one
UPDATE mentorship_matches
SET status = @status, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at;

-- name: DeleteMentorshipProgram :execrows
DELETE FROM mentorship_programs
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListMentorshipProgramsWithCounts :many
SELECT p.id, p.tenant_id, p.name, p.description, p.is_active, p.created_at, p.updated_at,
       COALESCE(mentor_counts.cnt, 0)::int AS mentor_count,
       COALESCE(mentee_counts.cnt, 0)::int AS mentee_count,
       COALESCE(match_counts.cnt, 0)::int AS match_count
FROM mentorship_programs p
LEFT JOIN (
    SELECT mm.program_id, COUNT(DISTINCT mm.mentor_id) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) mentor_counts ON mentor_counts.program_id = p.id
LEFT JOIN (
    SELECT mm.program_id, COUNT(DISTINCT mm.mentee_id) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) mentee_counts ON mentee_counts.program_id = p.id
LEFT JOIN (
    SELECT mm.program_id, COUNT(*) AS cnt
    FROM mentorship_matches mm WHERE mm.tenant_id = @tenant_id
    GROUP BY mm.program_id
) match_counts ON match_counts.program_id = p.id
WHERE p.tenant_id = @tenant_id
ORDER BY p.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- §102 Mentorship Matching queries (ADR-110)
-- Ref: japaconnect-engineering/sections/102-mentorship-matching.md
-- =====================================================================

-- name: GetMentorProfile :one
-- Get the current user's mentor profile settings from user_profiles.
SELECT up.user_id, up.tenant_id, up.is_mentor, up.mentor_capacity,
       up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
       up.display_name, up.avatar_url
FROM user_profiles up
WHERE up.tenant_id = @tenant_id AND up.user_id = @user_id;

-- name: UpdateMentorProfile :one
-- Toggle is_mentor and set capacity/areas/bio. When is_mentor flips to true,
-- mentor_since is set to now(). When false, mentor_since is cleared.
UPDATE user_profiles
SET is_mentor = @is_mentor,
    mentor_capacity = @mentor_capacity,
    mentor_areas = @mentor_areas,
    mentor_bio = @mentor_bio,
    mentor_since = CASE WHEN @is_mentor = true AND mentor_since IS NULL THEN now() ELSE mentor_since END,
    updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id
RETURNING user_id, tenant_id, is_mentor, mentor_capacity, active_mentee_count,
          mentor_areas, mentor_bio, mentor_since, display_name, avatar_url;

-- name: GetMentorByUsername :one
-- Get a mentor's public profile by username (is_mentor must be true).
SELECT up.user_id, up.tenant_id, up.is_mentor, up.mentor_capacity,
       up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
       up.display_name, up.avatar_url, u.username
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.tenant_id = @tenant_id AND u.username = @username AND up.is_mentor = true;

-- name: ListMentors :many
-- Discover mentors with optional destination and area filters.
-- dest: when provided, filters mentors whose mentor_areas mention the
--        destination country name, geo_code, slug, or any word of the name.
-- sort: 'newest' (mentor_since DESC), 'capacity' (available_slots DESC),
--        'match' (keyword area_overlap DESC — partial substring match).
WITH mentors AS (
    SELECT up.user_id, up.tenant_id, up.is_mentor, up.mentor_capacity,
           up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
           up.display_name, up.avatar_url, u.username,
           (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
           COUNT(*) OVER() AS total_count,
           COALESCE(
             (SELECT COUNT(*)::double precision / NULLIF(array_length(@areas::text[], 1), 0)
              FROM unnest(@areas::text[]) AS a
              WHERE EXISTS (
                  SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || a || '%'
              )),
             0
           ) AS area_overlap
    FROM user_profiles up
    JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
    WHERE up.tenant_id = @tenant_id
      AND up.is_mentor = true
      AND (
          @dest_space_id::uuid IS NULL OR EXISTS (
              SELECT 1 FROM spaces s
              WHERE s.id = @dest_space_id
                AND s.tenant_id = @tenant_id
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
              WHERE EXISTS (
                  SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || a || '%'
              )
          )
      )
)
SELECT user_id, tenant_id, is_mentor, mentor_capacity,
       active_mentee_count, mentor_areas, mentor_bio, mentor_since,
       display_name, avatar_url, username,
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
WITH mentors AS (
    SELECT up.user_id, up.tenant_id, up.is_mentor, up.mentor_capacity,
           up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
           up.display_name, up.avatar_url, u.username,
           (up.mentor_capacity - up.active_mentee_count)::int AS available_slots,
           COUNT(*) OVER() AS total_count,
           CASE WHEN up.mentor_embedding IS NOT NULL
                THEN (1 - (up.mentor_embedding <=> @mentee_embedding))::double precision
                ELSE 0::double precision
           END AS bio_similarity,
           COALESCE(
             (SELECT COUNT(*)::double precision / NULLIF(array_length(@areas::text[], 1), 0)
              FROM unnest(@areas::text[]) AS a
              WHERE EXISTS (
                  SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || a || '%'
              )),
             0
           ) AS area_overlap
    FROM user_profiles up
    JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
    WHERE up.tenant_id = @tenant_id
      AND up.is_mentor = true
      AND (
          @dest_space_id::uuid IS NULL OR EXISTS (
              SELECT 1 FROM spaces s
              WHERE s.id = @dest_space_id
                AND s.tenant_id = @tenant_id
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
              WHERE EXISTS (
                  SELECT 1 FROM unnest(up.mentor_areas) AS ma WHERE ma ILIKE '%' || a || '%'
              )
          )
      )
)
SELECT user_id, tenant_id, is_mentor, mentor_capacity,
       active_mentee_count, mentor_areas, mentor_bio, mentor_since,
       display_name, avatar_url, username,
       available_slots, total_count, bio_similarity, area_overlap,
       (0.6 * area_overlap + 0.4 * bio_similarity) AS match_score
FROM mentors
ORDER BY match_score DESC, mentor_since DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: GetMentorsByIDs :many
-- Get up to 4 mentors by user IDs for the comparison endpoint (§30).
SELECT up.user_id, up.tenant_id, up.is_mentor, up.mentor_capacity,
       up.active_mentee_count, up.mentor_areas, up.mentor_bio, up.mentor_since,
       up.display_name, up.avatar_url, u.username,
       (up.mentor_capacity - up.active_mentee_count)::int AS available_slots
FROM user_profiles up
JOIN users u ON u.id = up.user_id AND u.tenant_id = up.tenant_id
WHERE up.tenant_id = @tenant_id
  AND up.is_mentor = true
  AND up.user_id = ANY(@user_ids::uuid[])
ORDER BY u.username;

-- name: CreateMentorshipConnection :one
-- Create a new mentorship request (status = 'pending').
-- The UNIQUE(mentor_id, mentee_id, status) constraint prevents duplicate
-- pending requests between the same pair.
INSERT INTO mentorship_connections (tenant_id, mentor_id, mentee_id, status, goals, destination_space_id)
VALUES (@tenant_id, @mentor_id, @mentee_id, 'pending', @goals, @destination_space_id)
RETURNING *;

-- name: GetActiveOrPendingConnection :one
-- Check whether an active or pending connection already exists between a
-- mentor and mentee (§102: a new request can only be sent after the previous
-- one has been resolved). Returns at most one row.
SELECT id FROM mentorship_connections
WHERE tenant_id = @tenant_id
  AND mentor_id = @mentor_id
  AND mentee_id = @mentee_id
  AND status IN ('pending', 'active')
LIMIT 1;

-- name: GetMentorshipConnection :one
-- Get a single connection by ID (within tenant).
SELECT * FROM mentorship_connections
WHERE tenant_id = @tenant_id AND id = @id;

-- name: ListOutgoingConnections :many
-- List requests sent by the mentee (status filter optional).
SELECT mc.*, u.username AS mentor_username, up.display_name AS mentor_display_name
FROM mentorship_connections mc
JOIN users u ON u.id = mc.mentor_id
LEFT JOIN user_profiles up ON up.user_id = mc.mentor_id AND up.tenant_id = mc.tenant_id
WHERE mc.tenant_id = @tenant_id AND mc.mentee_id = @mentee_id
  AND (@status::text = '' OR mc.status = @status)
ORDER BY mc.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListIncomingConnections :many
-- List requests received by the mentor (status filter optional).
SELECT mc.*, u.username AS mentee_username, up.display_name AS mentee_display_name
FROM mentorship_connections mc
JOIN users u ON u.id = mc.mentee_id
LEFT JOIN user_profiles up ON up.user_id = mc.mentee_id AND up.tenant_id = mc.tenant_id
WHERE mc.tenant_id = @tenant_id AND mc.mentor_id = @mentor_id
  AND (@status::text = '' OR mc.status = @status)
ORDER BY mc.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AcceptMentorshipConnection :one
-- Mentor accepts a pending request (pending → active).
UPDATE mentorship_connections
SET status = 'active', responded_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'pending'
RETURNING *;

-- name: DeclineMentorshipConnection :one
-- Mentor declines a pending request (pending → declined).
UPDATE mentorship_connections
SET status = 'declined', responded_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'pending'
RETURNING *;

-- name: CompleteMentorshipConnection :one
-- Either party completes an active connection (active → completed).
UPDATE mentorship_connections
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'active'
RETURNING *;

-- name: CancelMentorshipConnection :one
-- Either party cancels an active connection (active → cancelled).
UPDATE mentorship_connections
SET status = 'cancelled', cancelled_at = now(), cancel_reason = @cancel_reason, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND status = 'active'
RETURNING *;

-- name: CancelPendingConnection :execrows
-- Mentee cancels their own pending request (pending → cancelled).
UPDATE mentorship_connections
SET status = 'cancelled', cancelled_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id AND mentee_id = @mentee_id AND status = 'pending';

-- name: ListAllConnections :many
-- Admin: list all connections with optional status/mentor/mentee filters.
SELECT mc.*, mu.username AS mentor_username, eu.username AS mentee_username
FROM mentorship_connections mc
LEFT JOIN users mu ON mu.id = mc.mentor_id
LEFT JOIN users eu ON eu.id = mc.mentee_id
WHERE mc.tenant_id = @tenant_id
  AND (@status::text = '' OR mc.status = @status)
  AND (@mentor_id::uuid IS NULL OR mc.mentor_id = @mentor_id)
  AND (@mentee_id::uuid IS NULL OR mc.mentee_id = @mentee_id)
ORDER BY mc.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminUpdateConnection :one
-- Admin override: set connection status (remove bad actor, force-complete, etc.).
UPDATE mentorship_connections
SET status = @status, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- name: UpdateMentorEmbedding :exec
-- Set the pgvector embedding for a mentor's bio (called by Asynq task).
UPDATE user_profiles
SET mentor_embedding = @embedding, updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetMentorEmbedding :one
-- Get a mentor's embedding vector (for match scoring).
SELECT user_id, mentor_embedding
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id AND is_mentor = true;

-- name: GetUserEmbedding :one
-- Get any user's mentor_embedding vector (for match scoring on the discovery
-- page). Unlike GetMentorEmbedding, this does NOT require is_mentor = true —
-- a mentee who is also a mentor will have an embedding from their own bio.
-- Returns NULL for users without an embedding (service falls back to
-- keyword-only scoring, §102).
SELECT user_id, mentor_embedding
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: GetMenteeDestinationSpaceID :one
-- Get the authenticated user's destination space ID (japa_stage_dest_space_id,
-- §87) for use as the default ?dest= filter on the mentor discovery page
-- (§102 Geo Awareness: "The mentee's profile destination_country_id is
-- injected as a default filter when logged in, with no UI action required").
SELECT user_id, japa_stage_dest_space_id AS destination_space_id
FROM user_profiles
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: ListMentorsWithEmbeddings :many
-- Get all mentors with their embeddings for batch Meilisearch sync.
SELECT up.user_id, up.mentor_embedding, up.is_mentor, up.mentor_areas,
       up.mentor_bio, up.mentor_capacity, up.active_mentee_count, up.mentor_since
FROM user_profiles up
WHERE up.tenant_id = @tenant_id AND up.is_mentor = true;
