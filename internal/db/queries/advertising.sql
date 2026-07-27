-- Advertising — campaigns, slot types, events, analytics, organization boosts.

-- =====================
-- Ad Slot Types
-- =====================

-- name: CreateAdSlotType :one
INSERT INTO ad_slot_types (
    tenant_id, slot_name, description, max_display_per_page
) VALUES (
    @tenant_id, @slot_name, sqlc.narg(description), @max_display_per_page
)
RETURNING *;

-- name: GetAdSlotTypeByID :one
SELECT * FROM ad_slot_types
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetAdSlotTypeByName :one
SELECT * FROM ad_slot_types
WHERE tenant_id = @tenant_id AND slot_name = @slot_name;

-- name: ListAdSlotTypes :many
SELECT * FROM ad_slot_types
WHERE tenant_id = @tenant_id
ORDER BY slot_name
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveAdSlotTypes :many
SELECT * FROM ad_slot_types
WHERE tenant_id = @tenant_id AND is_active = true
ORDER BY slot_name;

-- name: UpdateAdSlotType :one
UPDATE ad_slot_types
SET max_display_per_page = @max_display_per_page,
    is_active = @is_active
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: UpdateAdSlotTypeDetails :one
UPDATE ad_slot_types
SET slot_name = @slot_name, description = @description
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteAdSlotType :exec
DELETE FROM ad_slot_types WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Ad Campaigns
-- =====================

-- name: CreateAdCampaign :one
INSERT INTO ad_campaigns (
    tenant_id, name, slot_type_id, content, target_url,
    budget, status, priority, frequency_cap_per_user,
    start_date, end_date, created_by, target_geo, target_space_ids
) VALUES (
    @tenant_id, @name, @slot_type_id, @content, sqlc.narg(target_url),
    sqlc.narg(budget), @status, @priority, sqlc.narg(frequency_cap_per_user),
    sqlc.narg(start_date), sqlc.narg(end_date), sqlc.narg(created_by),
    sqlc.narg(target_geo), sqlc.narg(target_space_ids)
)
RETURNING *;

-- name: GetAdCampaignByID :one
SELECT * FROM ad_campaigns
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListAdCampaignsByTenant :many
SELECT * FROM ad_campaigns
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveCampaignsBySlot :many
SELECT * FROM ad_campaigns
WHERE tenant_id = @tenant_id
  AND slot_type_id = @slot_type_id
  AND status = 'active'
  AND (start_date IS NULL OR start_date <= now())
  AND (end_date IS NULL OR end_date >= now())
ORDER BY priority DESC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAdCampaignStatus :one
UPDATE ad_campaigns
SET status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: UpdateAdCampaign :one
UPDATE ad_campaigns
SET name = @name,
    content = @content,
    target_url = @target_url,
    budget = @budget,
    frequency_cap_per_user = @frequency_cap_per_user,
    start_date = @start_date,
    end_date = @end_date,
    target_geo = sqlc.narg(target_geo),
    target_space_ids = sqlc.narg(target_space_ids),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: IncrementCampaignSpent :exec
UPDATE ad_campaigns
SET spent = spent + @amount, updated_at = now()
WHERE id = @id;

-- name: DeleteAdCampaign :exec
DELETE FROM ad_campaigns
WHERE id = @id AND tenant_id = @tenant_id AND status IN ('draft', 'ended');

-- name: GetCampaignStats :one
SELECT
    COALESCE(SUM(impressions), 0)::bigint AS impressions,
    COALESCE(SUM(clicks), 0)::bigint AS clicks,
    COUNT(DISTINCT user_id)::bigint AS unique_users,
    COALESCE(SUM(conversions), 0)::bigint AS conversions
FROM ad_events
WHERE campaign_id = @campaign_id AND tenant_id = @tenant_id;

-- =====================
-- Ad Events (batched ingestion)
-- =====================

-- name: InsertAdEvent :one
INSERT INTO ad_events (
    tenant_id, campaign_id, user_id, event_type, occurred_at
) VALUES (
    @tenant_id, @campaign_id, @user_id, @event_type, @occurred_at
)
RETURNING *;

-- name: BatchInsertAdEvents :copyfrom
INSERT INTO ad_events (
    tenant_id, campaign_id, user_id, event_type, occurred_at
) VALUES (
    @tenant_id, @campaign_id, @user_id, @event_type, @occurred_at
);

-- name: CountImpressionsForUserToday :one
SELECT COUNT(*)::int FROM ad_events
WHERE user_id = @user_id
  AND campaign_id = @campaign_id
  AND event_type = 'impression'
  AND occurred_at >= current_date;

-- name: ListAdEventsByCampaign :many
SELECT * FROM ad_events
WHERE campaign_id = @campaign_id
ORDER BY occurred_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Ad Analytics Daily
-- =====================

-- name: UpsertAdAnalyticsDaily :one
INSERT INTO ad_analytics_daily (
    tenant_id, campaign_id, slot_type_id, date, impressions, clicks, spent
) VALUES (
    @tenant_id, @campaign_id, @slot_type_id, @date, @impressions, @clicks, @spent
)
ON CONFLICT (tenant_id, campaign_id, slot_type_id, date)
DO UPDATE SET
    impressions = ad_analytics_daily.impressions + EXCLUDED.impressions,
    clicks      = ad_analytics_daily.clicks + EXCLUDED.clicks,
    spent       = ad_analytics_daily.spent + EXCLUDED.spent
RETURNING *;

-- name: GetAdAnalyticsByCampaign :many
SELECT * FROM ad_analytics_daily
WHERE campaign_id = @campaign_id
ORDER BY date DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetAdAnalyticsBySlot :many
SELECT * FROM ad_analytics_daily
WHERE tenant_id = @tenant_id AND slot_type_id = @slot_type_id
  AND date >= @date_from AND date <= @date_to
ORDER BY date DESC;

-- =====================
-- Organization Boosts
-- =====================

-- name: CreateOrganizationBoost :one
INSERT INTO organization_boosts (
    tenant_id, organization_id, campaign_id, boost_type,
    status, start_date, end_date
) VALUES (
    @tenant_id, @organization_id, @campaign_id, @boost_type,
    @status, @start_date, sqlc.narg(end_date)
)
RETURNING *;

-- name: GetOrganizationBoostByID :one
SELECT * FROM organization_boosts
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListOrganizationBoosts :many
SELECT * FROM organization_boosts
WHERE tenant_id = @tenant_id AND organization_id = @organization_id
ORDER BY created_at DESC;

-- name: ListActiveBoosts :many
SELECT * FROM organization_boosts
WHERE tenant_id = @tenant_id AND status = 'active'
  AND (end_date IS NULL OR end_date >= now())
ORDER BY start_date DESC;

-- name: EndOrganizationBoost :one
UPDATE organization_boosts
SET status = 'ended', end_date = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: UpdateOrganizationBoost :one
UPDATE organization_boosts
SET status = sqlc.narg(status), end_date = sqlc.narg(end_date)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteOrganizationBoost :exec
DELETE FROM organization_boosts
WHERE id = @id AND tenant_id = @tenant_id;