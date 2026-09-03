-- campaigns.sql — Content Campaign queries (Section 104, ADR-104).
-- Covers campaign and campaign item CRUD, status transitions, and the
-- active persona pool lookups used during planning.
-- Ref: docs/superpowers/specs/2026-08-24-content-campaigns-design.md

-- =====================================================================
-- Campaigns
-- =====================================================================

-- name: CreateContentCampaign :one
INSERT INTO content_campaigns (
    tenant_id, name, prompt, status, content_type, requested_count,
    persona_pool, auto_provision_personas, auto_assign,
    target_space_id, target_tags, plan_json, created_by
) VALUES (
    @tenant_id, @name, @prompt, @status, sqlc.narg(content_type), @requested_count,
    sqlc.narg(persona_pool), @auto_provision_personas, @auto_assign,
    sqlc.narg(target_space_id), sqlc.narg(target_tags), sqlc.narg(plan_json), @created_by
)
RETURNING *;

-- name: GetContentCampaign :one
SELECT * FROM content_campaigns
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListContentCampaigns :many
SELECT * FROM content_campaigns
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountContentCampaigns :one
SELECT count(*) FROM content_campaigns
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- name: UpdateContentCampaignStatus :one
UPDATE content_campaigns
SET status = @status,
    plan_json = COALESCE(sqlc.narg(plan_json), plan_json),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- =====================================================================
-- Campaign items
-- =====================================================================

-- name: CreateContentCampaignItem :one
INSERT INTO content_campaign_items (
    tenant_id, campaign_id, sort_order, persona_id, content_type, topic_prompt,
    title, target_space_id, target_tags, status
) VALUES (
    @tenant_id, @campaign_id, @sort_order, @persona_id, @content_type, @topic_prompt,
    sqlc.narg(title), sqlc.narg(target_space_id), sqlc.narg(target_tags), 'planned'
)
RETURNING *;

-- name: GetContentCampaignItem :one
SELECT * FROM content_campaign_items
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetContentCampaignItemByStudioJob :one
SELECT * FROM content_campaign_items
WHERE studio_job_id = @studio_job_id AND tenant_id = @tenant_id;

-- name: GetContentCampaignItemByReviewID :one
SELECT * FROM content_campaign_items
WHERE review_id = @review_id AND tenant_id = @tenant_id;

-- name: ListContentCampaignItemsByCampaign :many
SELECT * FROM content_campaign_items
WHERE campaign_id = @campaign_id AND tenant_id = @tenant_id
ORDER BY sort_order, created_at;

-- name: UpdateContentCampaignItem :one
UPDATE content_campaign_items
SET status = @status,
    studio_job_id = COALESCE(sqlc.narg(studio_job_id), studio_job_id),
    content_id = COALESCE(sqlc.narg(content_id), content_id),
    review_id = COALESCE(sqlc.narg(review_id), review_id),
    error_message = sqlc.narg(error_message)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- =====================================================================
-- Active persona pool used for planning and validation
-- =====================================================================

-- name: ListContentCampaignActivePersonas :many
SELECT ap.id, COALESCE(up.display_name, u.username) AS profile_name, COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = ap.user_id AND up.tenant_id = ap.tenant_id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
WHERE ap.tenant_id = @tenant_id AND ap.status = 'active'
ORDER BY COALESCE(up.display_name, u.username);

-- name: ListContentCampaignActivePersonasByIDs :many
SELECT ap.id, COALESCE(up.display_name, u.username) AS profile_name, COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = ap.user_id AND up.tenant_id = ap.tenant_id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
WHERE ap.tenant_id = @tenant_id AND ap.status = 'active'
  AND ap.id = ANY(@ids::uuid[])
ORDER BY COALESCE(up.display_name, u.username);
