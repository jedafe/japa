-- personas.sql — AI Persona System queries (Section 68, ADR-068).
-- Covers transactional mass provisioning (users + professional_profiles +
-- ai_personas in one tx), CRUD, and activity log retrieval.
-- Ref: japaconnect-engineering/sections/68-ai-personas.md
--      japaconnect-engineering/adr/ADR-068-ai-persona-system.md

-- =====================================================================
-- Create a persona backing account (is_bot = true). Used inside the
-- transactional provisioning flow — the caller wraps this with profile
-- and config creation in a single tx.
-- =====================================================================

-- name: CreatePersonaUser :one
INSERT INTO users (tenant_id, email, username, profile_type, status, is_bot)
VALUES (@tenant_id, @email, @username, 'professional', 'active', true)
RETURNING *;

-- =====================================================================
-- Create the ai_personas config row linking the backing user to its
-- professional profile byline.
-- =====================================================================

-- name: CreateAIPersona :one
INSERT INTO ai_personas (
    tenant_id, user_id, profile_id, disclosure_mode, autonomy_mode,
    voice_prompt, activity_policy, status, created_by
)
VALUES (
    @tenant_id, @user_id, @profile_id, @disclosure_mode, @autonomy_mode,
    sqlc.narg(voice_prompt), sqlc.narg(activity_policy), @status, @created_by
)
RETURNING *;

-- =====================================================================
-- Retrieve a single persona by ID (with joined user + profile names).
-- =====================================================================

-- name: GetAIPersonaByID :one
SELECT
    ap.*,
    u.username  AS backing_username,
    u.email     AS backing_email,
    pp.name     AS profile_name,
    pp.slug     AS profile_slug,
    pp.tagline  AS profile_tagline
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
JOIN professional_profiles pp ON pp.id = ap.profile_id
WHERE ap.tenant_id = @tenant_id AND ap.id = @id;

-- =====================================================================
-- List personas for a tenant with pagination.
-- =====================================================================

-- name: ListAIPersonas :many
SELECT
    ap.*,
    u.username  AS backing_username,
    pp.name     AS profile_name
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
JOIN professional_profiles pp ON pp.id = ap.profile_id
WHERE ap.tenant_id = @tenant_id
  AND (@status::text = '' OR ap.status = @status)
ORDER BY ap.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAIPersonas :one
SELECT COUNT(*) AS count
FROM ai_personas
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- =====================================================================
-- Update a persona's config (activity_policy, modes, status, voice_prompt).
-- =====================================================================

-- name: UpdateAIPersona :one
UPDATE ai_personas
SET
    disclosure_mode = COALESCE(sqlc.narg(disclosure_mode), disclosure_mode),
    autonomy_mode   = COALESCE(sqlc.narg(autonomy_mode), autonomy_mode),
    voice_prompt    = COALESCE(sqlc.narg(voice_prompt), voice_prompt),
    activity_policy = COALESCE(sqlc.narg(activity_policy), activity_policy),
    status          = COALESCE(sqlc.narg(status), status),
    updated_at      = now()
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- =====================================================================
-- Get a persona by its backing user_id (used by the orchestration engine
-- to resolve the persona config from the actor identity).
-- =====================================================================

-- name: GetAIPersonaByUserID :one
SELECT * FROM ai_personas
WHERE tenant_id = @tenant_id AND user_id = @user_id;

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
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND is_synthetic = true
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPersonaActivity :one
SELECT COUNT(*) AS count
FROM platform_events
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND is_synthetic = true;
