-- personas.sql — AI Persona System queries (Section 68, ADR-068, ADR-114).
-- Covers transactional mass provisioning (users + user_profiles +
-- ai_personas + persona_voices in one tx), CRUD, activity log retrieval,
-- and shared voice management.
-- Ref: japaconnect-engineering/sections/68-ai-personas.md
--      japaconnect-engineering/adr/ADR-068-ai-persona-system.md
--      japaconnect-engineering/adr/ADR-114-shared-persona-voices-and-content-type-model-defaults.md

-- =====================================================================
-- Create a persona backing account (is_bot = true). Used inside the
-- transactional provisioning flow — the caller wraps this with profile
-- and config creation in a single tx.
-- =====================================================================

-- name: CreatePersonaUser :one
INSERT INTO users (email, username, profile_type, status, is_bot) VALUES (@email, @username, 'regular', 'active', true)

RETURNING *;

-- =====================================================================
-- Create the ai_personas config row linking the backing user to its
-- user profile byline. profile_id is retained but nullable for backward
-- compatibility after the professional profile removal (§33).
-- =====================================================================

-- name: CreateAIPersona :one
INSERT INTO ai_personas (user_id, profile_id, disclosure_mode, autonomy_mode, voice_id, voice_prompt, activity_policy, status, created_by) VALUES (@user_id, sqlc.narg(profile_id), @disclosure_mode, @autonomy_mode, sqlc.narg(voice_id), sqlc.narg(voice_prompt), sqlc.narg(activity_policy), @status, @created_by)

RETURNING *;

-- =====================================================================
-- Retrieve a single persona by ID (with joined user + profile + voice).
-- Returns the resolved voice prompt (override wins) plus voice metadata.
-- =====================================================================

-- name: GetAIPersonaByID :one
-- Returns the persona with its bot user byline (from users + user_profiles).
SELECT
 ap.id,
 ap.user_id,
 ap.profile_id,
 ap.disclosure_mode,
 ap.autonomy_mode,
 ap.voice_id,
 ap.voice_prompt AS voice_prompt_override,
 COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt,
 ap.activity_policy,
 ap.status,
 ap.created_by,
 ap.created_at,
 ap.updated_at,
 u.username AS backing_username,
 u.email AS backing_email,
 COALESCE(up.display_name, u.username) AS profile_name,
 u.username AS profile_slug,
 up.bio AS profile_tagline,
 pv.name AS voice_name,
 pv.slug AS voice_slug
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
WHERE ap.id = @id;

-- =====================================================================
-- List personas for a tenant with pagination.
-- =====================================================================

-- name: ListAIPersonas :many
SELECT
 ap.id,
 ap.user_id,
 ap.profile_id,
 ap.disclosure_mode,
 ap.autonomy_mode,
 ap.voice_id,
 ap.voice_prompt AS voice_prompt_override,
 COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt,
 ap.activity_policy,
 ap.status,
 ap.created_by,
 ap.created_at,
 ap.updated_at,
 u.username AS backing_username,
 COALESCE(up.display_name, u.username) AS profile_name,
 pv.name AS voice_name,
 stats.total_actions AS total_actions,
 stats.last_action_at AS last_action_at
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
LEFT JOIN LATERAL (
 SELECT COUNT(*) AS total_actions, MAX(pe.created_at) AS last_action_at
 FROM platform_events pe
 WHERE pe.user_id = ap.user_id
 AND pe.is_synthetic = true
) stats ON true
WHERE (@status::text = '' OR ap.status = @status)
 AND (@q::text = '' OR COALESCE(up.display_name, u.username) ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%')
ORDER BY
 CASE WHEN @sort::text = 'name' AND @dir::text = 'asc' THEN COALESCE(up.display_name, u.username) END ASC,
 CASE WHEN @sort::text = 'name' AND @dir::text = 'desc' THEN COALESCE(up.display_name, u.username) END DESC,
 CASE WHEN @sort::text = 'created_at' AND @dir::text = 'asc' THEN ap.created_at END ASC,
 CASE WHEN @sort::text = 'created_at' AND @dir::text = 'desc' THEN ap.created_at END DESC,
 CASE WHEN @sort::text = 'total_actions' AND @dir::text = 'asc' THEN stats.total_actions END ASC,
 CASE WHEN @sort::text = 'total_actions' AND @dir::text = 'desc' THEN stats.total_actions END DESC,
 ap.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAIPersonas :one
SELECT COUNT(*) AS count
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
WHERE (@status::text = '' OR ap.status = @status)
 AND (@q::text = '' OR COALESCE(up.display_name, u.username) ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%');

-- =====================================================================
-- Update a persona's config (activity_policy, modes, status, voice).
-- =====================================================================

-- name: UpdateAIPersona :one
UPDATE ai_personas
SET
 disclosure_mode = COALESCE(sqlc.narg(disclosure_mode), disclosure_mode),
 autonomy_mode = COALESCE(sqlc.narg(autonomy_mode), autonomy_mode),
 voice_id = CASE
 WHEN sqlc.narg(voice_id) IS NULL THEN voice_id
 WHEN sqlc.narg(voice_id) = '00000000-0000-0000-0000-000000000000'::uuid THEN NULL
 ELSE sqlc.narg(voice_id)
 END,
 voice_prompt = COALESCE(sqlc.narg(voice_prompt), voice_prompt),
 activity_policy = COALESCE(sqlc.narg(activity_policy), activity_policy),
 status = COALESCE(sqlc.narg(status), status),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- =====================================================================
-- Get a persona by its backing user_id (used by the orchestration engine
-- to resolve the persona config from the actor identity).
-- Returns the resolved voice prompt.
-- =====================================================================

-- name: GetAIPersonaByUserID :one
SELECT
 ap.id,
 ap.user_id,
 ap.profile_id,
 ap.disclosure_mode,
 ap.autonomy_mode,
 ap.voice_id,
 ap.voice_prompt AS voice_prompt_override,
 COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt,
 ap.activity_policy,
 ap.status,
 ap.created_by,
 ap.created_at,
 ap.updated_at
FROM ai_personas ap
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
WHERE ap.user_id = @user_id;

-- =====================================================================
-- Activity log — recent platform_events for a persona's backing user.
-- =====================================================================

-- name: ListPersonaActivity :many
SELECT
 id,
 event_type,
 entity_type,
 entity_id,
 meta,
 is_synthetic,
 created_at
FROM platform_events
WHERE user_id = @user_id
 AND is_synthetic = true
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPersonaActivity :one
SELECT COUNT(*) AS count
FROM platform_events
WHERE user_id = @user_id
 AND is_synthetic = true;

-- =====================================================================
-- Shared persona voices (ADR-114).
-- =====================================================================

-- name: CreatePersonaVoice :one
INSERT INTO persona_voices (name, slug, description, voice_prompt, is_system, created_by) VALUES (@name, @slug, sqlc.narg(description), @voice_prompt, @is_system, @created_by)

RETURNING *;

-- name: GetPersonaVoiceByID :one
SELECT * FROM persona_voices
WHERE id = @id;

-- name: GetPersonaVoiceBySlug :one
SELECT * FROM persona_voices
WHERE slug = @slug;

-- name: ListPersonaVoices :many
SELECT * FROM persona_voices

ORDER BY name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPersonaVoices :one
SELECT COUNT(*) AS count
FROM persona_voices
;

-- name: UpdatePersonaVoice :one
UPDATE persona_voices
SET
 name = COALESCE(sqlc.narg(name), name),
 slug = COALESCE(sqlc.narg(slug), slug),
 description = COALESCE(sqlc.narg(description), description),
 voice_prompt = COALESCE(sqlc.narg(voice_prompt), voice_prompt),
 is_system = COALESCE(sqlc.narg(is_system), is_system),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeletePersonaVoice :exec
DELETE FROM persona_voices
WHERE id = @id;
