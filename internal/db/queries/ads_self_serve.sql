-- ads_self_serve.sql — Self-Serve Advertising queries (Section 61, ADR-079).
-- Covers org-facing campaign CRUD, slot listing, and campaign stats.
-- Ref: japaconnect-engineering/sections/61-self-serve-ads.md
--      japaconnect-engineering/adr/ADR-079-self-serve-ads.md

-- =====================================================================
-- List available ad slot types with flat_rate for self-serve advertisers.
-- =====================================================================

-- name: ListAdSlotTypesForSelfServe :many
SELECT * FROM ad_slot_types
WHERE is_active = true
ORDER BY slot_name ASC;

-- =====================================================================
-- Create a self-serve ad campaign (org-funded, pending review).
-- =====================================================================

-- name: CreateSelfServeAdCampaign :one
INSERT INTO ad_campaigns (organization_id, name, slot_type_id, content, target_url, budget_credits, flat_rate, status, priority, frequency_cap_per_user, start_date, end_date, target_geo, target_space_ids, created_by) VALUES (@organization_id, @name, @slot_type_id, @content, sqlc.narg(target_url), @budget_credits, @flat_rate, 'pending_review', 0, sqlc.narg(frequency_cap_per_user), sqlc.narg(start_date), sqlc.narg(end_date), @target_geo, @target_space_ids, @created_by)

RETURNING *;

-- =====================================================================
-- Get a single campaign by ID (org-scoped).
-- =====================================================================

-- name: GetSelfServeAdCampaign :one
SELECT * FROM ad_campaigns
WHERE id = @id AND organization_id = @organization_id;

-- =====================================================================
-- List an org's campaigns.
-- =====================================================================

-- name: ListOrgAdCampaigns :many
SELECT * FROM ad_campaigns
WHERE organization_id = @organization_id
 AND (@status::text = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountOrgAdCampaigns :one
SELECT COUNT(*) AS count
FROM ad_campaigns
WHERE organization_id = @organization_id
 AND (@status::text = '' OR status = @status);

-- =====================================================================
-- Update a campaign's creative (only when pending_review or paused).
-- =====================================================================

-- name: UpdateSelfServeAdCampaignCreative :one
UPDATE ad_campaigns
SET
 content = COALESCE(sqlc.narg(content), content),
 target_url = COALESCE(sqlc.narg(target_url), target_url),
 updated_at = now()
WHERE id = @id AND organization_id = @organization_id
 AND status IN ('pending_review', 'paused')
RETURNING *;

-- =====================================================================
-- Pause an active campaign.
-- =====================================================================

-- name: PauseAdCampaign :one
UPDATE ad_campaigns
SET status = 'paused', updated_at = now()
WHERE id = @id AND organization_id = @organization_id
 AND status = 'active'
RETURNING *;

-- =====================================================================
-- Resume a paused campaign.
-- =====================================================================

-- name: ResumeAdCampaign :one
UPDATE ad_campaigns
SET status = 'active', updated_at = now()
WHERE id = @id AND organization_id = @organization_id
 AND status = 'paused'
RETURNING *;

-- =====================================================================
-- Campaign stats — impressions, clicks, CTR, spend.
-- =====================================================================

-- name: GetAdCampaignStats :one
SELECT
 COUNT(*) FILTER (WHERE ae.event_type = 'impression') AS impressions,
 COUNT(*) FILTER (WHERE ae.event_type = 'click') AS clicks,
 ac.spent,
 ac.budget_credits
FROM ad_campaigns ac
LEFT JOIN ad_events ae ON ae.campaign_id = ac.id
WHERE ac.id = @id AND ac.organization_id = @organization_id
GROUP BY ac.spent, ac.budget_credits;

-- =====================================================================
-- Check if an organization is verified (eligibility gate).
-- =====================================================================

-- name: IsOrgVerified :one
SELECT verified FROM organizations
WHERE id = @id;

-- =====================================================================
-- Check if a user is a member of an organization (org admin check).
-- =====================================================================

-- name: IsOrgMember :one
SELECT EXISTS(
 SELECT 1 FROM organization_members
 WHERE organization_id = @org_id AND user_id = @user_id
) AS is_member;

-- =====================================================================
-- Refund unused credits on rejection or early end.
-- =====================================================================

-- name: GetCampaignForRefund :one
SELECT * FROM ad_campaigns
WHERE id = @id AND organization_id IS NOT NULL;

-- name: MarkCampaignRefunded :exec
UPDATE ad_campaigns
SET budget_credits = 0, status = 'rejected', updated_at = now()
WHERE id = @id;
