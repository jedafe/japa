-- Advertising — campaigns, slot types, events, analytics, organization boosts.

-- =====================
-- Ad Slot Types
-- =====================

-- name: CreateAdSlotType :one
INSERT INTO ad_slot_types (slot_name, description, max_display_per_page) VALUES (@slot_name, sqlc.narg(description), @max_display_per_page)
RETURNING *;

-- name: GetAdSlotTypeByID :one
SELECT * FROM ad_slot_types
WHERE id = @id;

-- name: GetAdSlotTypeByName :one
SELECT * FROM ad_slot_types
WHERE slot_name = @slot_name;

-- name: ListAdSlotTypes :many
SELECT * FROM ad_slot_types

ORDER BY slot_name
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveAdSlotTypes :many
SELECT * FROM ad_slot_types
WHERE is_active = true
ORDER BY slot_name;

-- name: UpdateAdSlotType :one
UPDATE ad_slot_types
SET max_display_per_page = @max_display_per_page,
 is_active = @is_active
WHERE id = @id
RETURNING *;

-- name: UpdateAdSlotTypeDetails :one
UPDATE ad_slot_types
SET slot_name = @slot_name, description = @description
WHERE id = @id
RETURNING *;

-- name: DeleteAdSlotType :exec
DELETE FROM ad_slot_types WHERE id = @id;

-- =====================
-- Ad Campaigns
-- =====================

-- name: CreateAdCampaign :one
INSERT INTO ad_campaigns (name, slot_type_id, content, target_url, budget, status, priority, frequency_cap_per_user, start_date, end_date, created_by, target_geo, target_space_ids, is_house) VALUES (@name, @slot_type_id, @content, sqlc.narg(target_url), sqlc.narg(budget), @status, @priority, sqlc.narg(frequency_cap_per_user), sqlc.narg(start_date), sqlc.narg(end_date), sqlc.narg(created_by), sqlc.narg(target_geo), sqlc.narg(target_space_ids), @is_house)
RETURNING *;

-- name: GetAdCampaignByID :one
SELECT * FROM ad_campaigns
WHERE id = @id;

-- name: ListAdCampaigns :many
SELECT * FROM ad_campaigns

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveCampaignsBySlot :many
SELECT * FROM ad_campaigns
WHERE slot_type_id = @slot_type_id
 AND status = 'active'
 AND (start_date IS NULL OR start_date <= now())
 AND (end_date IS NULL OR end_date >= now())
ORDER BY priority DESC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListEligibleCampaignsForServing :many
-- Ref: sections/25-ad-placement.md (Amendment — serving targeting + house tier).
-- Two-pass serving calls this twice per request: is_house=false first, then
-- is_house=true as fallback fill when no paid/self-serve campaign is eligible.
-- NULL user_country / space_id params exclude geo- and space-targeted rows.
SELECT * FROM ad_campaigns
WHERE slot_type_id = @slot_type_id
 AND status = 'active'
 AND is_house = @is_house
 AND (start_date IS NULL OR start_date <= now())
 AND (end_date IS NULL OR end_date >= now())
 AND (coalesce(cardinality(target_geo), 0) = 0 OR sqlc.narg(user_country)::text = ANY(target_geo))
 AND (coalesce(cardinality(target_space_ids), 0) = 0 OR sqlc.narg(space_id)::uuid = ANY(target_space_ids))
ORDER BY priority DESC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveFlatRateCampaigns :many
-- For ad:accrue_spend_daily — active campaigns that accrue daily spend
-- (flat_rate > 0) plus any active campaign with an end_date so the sweep can
-- still auto-end flat_rate=0 stragglers.
SELECT * FROM ad_campaigns
WHERE status = 'active'
 AND (flat_rate > 0 OR end_date IS NOT NULL);

-- name: AccrueCampaignSpend :one
-- spent += flat_rate. Per-campaign idempotency (§25): campaigns with
-- flat_rate>0 accrue at most once per UTC day via last_accrual_date —
-- ErrNoRows means "already accrued today". flat_rate=0 rows always
-- return (the +0 write is a no-op) so the end-of-campaign sweep still
-- sees them on every run, including retries.
UPDATE ad_campaigns
SET spent = spent + flat_rate,
    last_accrual_date = CURRENT_DATE,
    updated_at = now()
WHERE id = @id
 AND (flat_rate = 0 OR last_accrual_date IS NULL OR last_accrual_date < CURRENT_DATE)
RETURNING *;

-- name: UpdateAdCampaignStatus :one
UPDATE ad_campaigns
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdateAdCampaignDetails :one
-- Partial PATCH for the admin UI: only name and budget are editable from
-- the campaign row. ad_campaigns has no description column, so it is
-- intentionally omitted. Fields use COALESCE so callers can omit either.
UPDATE ad_campaigns
SET name = COALESCE(NULLIF(@name::text, ''), name),
 budget = COALESCE(@budget, budget),
 is_house = COALESCE(sqlc.narg(is_house), is_house),
 updated_at = now()
WHERE id = @id
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
WHERE id = @id
RETURNING *;

-- name: IncrementCampaignSpent :exec
UPDATE ad_campaigns
SET spent = spent + @amount, updated_at = now()
WHERE id = @id;

-- name: DeleteAdCampaign :exec
DELETE FROM ad_campaigns
WHERE id = @id AND status IN ('draft', 'ended');

-- name: GetCampaignStats :one
-- Ref: japaconnect-engineering/sections/25-ad-placement.md
-- ad_events stores one row per impression/click (event_type discriminator),
-- NOT aggregate counters. Aggregate by counting rows per event_type.
-- Join ad_campaigns to read the spent column for spend/CPC computation.
SELECT
 COUNT(*) FILTER (WHERE e.event_type = 'impression')::bigint AS impressions,
 COUNT(*) FILTER (WHERE e.event_type = 'click')::bigint AS clicks,
 COUNT(DISTINCT e.user_id)::bigint AS unique_users,
 COUNT(*) FILTER (WHERE e.event_type = 'conversion')::bigint AS conversions,
 COALESCE(c.spent, 0)::bigint AS spent
FROM ad_events e
LEFT JOIN ad_campaigns c ON c.id = e.campaign_id
WHERE e.campaign_id = @campaign_id;

-- =====================
-- Ad Events (batched ingestion)
-- =====================

-- name: InsertAdEvent :one
INSERT INTO ad_events (campaign_id, user_id, event_type, occurred_at) VALUES (@campaign_id, @user_id, @event_type, @occurred_at)
RETURNING *;

-- name: BatchInsertAdEvents :copyfrom
INSERT INTO ad_events (campaign_id, user_id, event_type, occurred_at) VALUES (@campaign_id, @user_id, @event_type, @occurred_at) ;

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
INSERT INTO ad_analytics_daily (campaign_id, slot_type_id, date, impressions, clicks, spent) VALUES (@campaign_id, @slot_type_id, @date, @impressions, @clicks, @spent)
ON CONFLICT (campaign_id, slot_type_id, date)
DO UPDATE SET
 impressions = ad_analytics_daily.impressions + EXCLUDED.impressions,
 clicks = ad_analytics_daily.clicks + EXCLUDED.clicks,
 spent = ad_analytics_daily.spent + EXCLUDED.spent
RETURNING *;

-- name: GetAdAnalyticsByCampaign :many
SELECT * FROM ad_analytics_daily
WHERE campaign_id = @campaign_id
ORDER BY date DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetAdAnalyticsBySlot :many
SELECT * FROM ad_analytics_daily
WHERE slot_type_id = @slot_type_id
 AND date >= @date_from AND date <= @date_to
ORDER BY date DESC;

-- =====================
-- Organization Boosts
-- =====================

-- name: CreateOrganizationBoost :one
INSERT INTO organization_boosts (organization_id, campaign_id, boost_type, status, start_date, end_date) VALUES (@organization_id, @campaign_id, @boost_type, @status, @start_date, sqlc.narg(end_date))
RETURNING *;

-- name: GetOrganizationBoostByID :one
SELECT * FROM organization_boosts
WHERE id = @id;

-- name: ListOrganizationBoosts :many
SELECT * FROM organization_boosts
WHERE organization_id = @organization_id
ORDER BY created_at DESC;

-- name: ListActiveBoosts :many
-- Ref: japaconnect-engineering/sections/25-ad-placement.md
-- Organization boosts link an organization to a campaign (not a space).
-- The previous query referenced ob.space_id which does not exist on
-- organization_boosts, causing a 500 on GET /admin/ads/boosts/active.
SELECT ob.*, o.name AS organization_name
FROM organization_boosts ob
LEFT JOIN organizations o ON o.id = ob.organization_id
WHERE ob.status = 'active'
 AND (ob.end_date IS NULL OR ob.end_date >= now())
ORDER BY ob.start_date DESC;

-- name: EndOrganizationBoost :one
UPDATE organization_boosts
SET status = 'ended', end_date = now()
WHERE id = @id
RETURNING *;

-- name: UpdateOrganizationBoost :one
UPDATE organization_boosts
SET status = sqlc.narg(status), end_date = sqlc.narg(end_date)
WHERE id = @id
RETURNING *;

-- name: DeleteOrganizationBoost :exec
DELETE FROM organization_boosts
WHERE id = @id;

-- =====================
-- Ad Networks (Section 25 — external ad network configuration)
-- =====================

-- name: CreateAdNetwork :one
INSERT INTO ad_networks (provider, name, client_id, api_key_encrypted, slot_mapping, config, is_active, priority) VALUES (@provider, @name, sqlc.narg(client_id), sqlc.narg(api_key_encrypted), @slot_mapping, @config, @is_active, @priority)
RETURNING *;

-- name: GetAdNetworkByID :one
SELECT * FROM ad_networks WHERE id = @id;

-- name: GetAdNetworkByProvider :one
SELECT * FROM ad_networks WHERE provider = @provider;

-- name: ListAdNetworks :many
SELECT * FROM ad_networks
ORDER BY priority DESC, name LIMIT @page_limit OFFSET @page_offset;

-- name: ListActiveAdNetworks :many
SELECT * FROM ad_networks WHERE is_active = true
ORDER BY priority DESC, name;

-- name: UpdateAdNetwork :one
UPDATE ad_networks
SET name = @name, client_id = sqlc.narg(client_id), api_key_encrypted = sqlc.narg(api_key_encrypted),
 slot_mapping = @slot_mapping, config = @config, is_active = @is_active, priority = @priority
WHERE id = @id RETURNING *;

-- name: DeleteAdNetwork :exec
DELETE FROM ad_networks WHERE id = @id;

-- name: CountAdNetworks :one
SELECT COUNT(*)::int FROM ad_networks ;