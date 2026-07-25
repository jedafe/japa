-- User Affiliate Program (Section 56)

-- =====================
-- Program Enrollment
-- =====================

-- name: CreateAffiliateMember :one
INSERT INTO affiliate_program_members (user_id, tenant_id, status)
VALUES (@user_id, @tenant_id, @status)
ON CONFLICT (user_id) DO UPDATE
    SET status = EXCLUDED.status
RETURNING user_id, tenant_id, enrolled_at, status, total_clicks,
    total_conversions, total_earned_credits, payout_method, notes;

-- name: GetAffiliateMember :one
SELECT user_id, tenant_id, enrolled_at, status, total_clicks,
    total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: GetAffiliateMemberByUserID :one
SELECT user_id, tenant_id, enrolled_at, status, total_clicks,
    total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE user_id = @user_id;

-- name: ListAffiliateMembers :many
SELECT user_id, tenant_id, enrolled_at, status, total_clicks,
    total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE tenant_id = @tenant_id
ORDER BY enrolled_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAffiliateMemberStatus :one
UPDATE affiliate_program_members
SET status = @status, notes = sqlc.narg(notes)
WHERE user_id = @user_id AND tenant_id = @tenant_id
RETURNING user_id, tenant_id, enrolled_at, status, total_clicks,
    total_conversions, total_earned_credits, payout_method, notes;

-- name: IncrementAffiliateMemberClicks :exec
UPDATE affiliate_program_members
SET total_clicks = total_clicks + 1
WHERE user_id = @user_id;

-- name: IncrementAffiliateMemberConversions :exec
UPDATE affiliate_program_members
SET total_conversions = total_conversions + 1,
    total_earned_credits = total_earned_credits + @commission
WHERE user_id = @user_id;

-- name: CountAffiliateMembersByTenant :one
SELECT COUNT(*) AS count
FROM affiliate_program_members
WHERE tenant_id = @tenant_id;

-- =====================
-- User Affiliate Links CRUD
-- =====================

-- name: CreateAffiliateLink :one
INSERT INTO user_affiliate_links (
    tenant_id, user_id, product_type, product_id, code, description, destination_url
) VALUES (
    @tenant_id, @user_id, @product_type, sqlc.narg(product_id), @code,
    sqlc.narg(description), @destination_url
)
RETURNING id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at;

-- name: GetAffiliateLinkByID :one
SELECT id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetAffiliateLinkByCode :one
SELECT id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE code = @code AND is_active = true;

-- name: ListAffiliateLinksByUser :many
SELECT id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE user_id = @user_id AND tenant_id = @tenant_id
    AND (@is_active::bool IS NULL OR is_active = @is_active)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAllAffiliateLinks :many
SELECT id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAffiliateLink :one
UPDATE user_affiliate_links
SET description = sqlc.narg(description), is_active = @is_active
WHERE id = @id AND user_id = @user_id AND tenant_id = @tenant_id
RETURNING id, tenant_id, user_id, product_type, product_id, code, description,
    destination_url, click_count, conversion_count, total_earned, is_active, created_at;

-- name: DeactivateAffiliateLink :exec
UPDATE user_affiliate_links
SET is_active = false
WHERE id = @id AND user_id = @user_id AND tenant_id = @tenant_id;

-- name: CountActiveLinksByUser :one
SELECT COUNT(*) AS count
FROM user_affiliate_links
WHERE user_id = @user_id AND is_active = true;

-- name: DeactivateLinksByMemberStatus :exec
UPDATE user_affiliate_links
SET is_active = false
WHERE user_id = @user_id AND is_active = true;

-- =====================
-- Affiliate Link Events
-- =====================

-- name: CreateAffiliateLinkEvent :one
INSERT INTO affiliate_link_events (
    tenant_id, link_id, event_type, referrer_url, ip_hash, user_agent_hash,
    converted_payment_id, commission_amount
) VALUES (
    @tenant_id, @link_id, @event_type, sqlc.narg(referrer_url), sqlc.narg(ip_hash),
    sqlc.narg(user_agent_hash), sqlc.narg(converted_payment_id), sqlc.narg(commission_amount)
)
RETURNING id, tenant_id, link_id, event_type, referrer_url, ip_hash,
    user_agent_hash, converted_payment_id, commission_amount, created_at;

-- name: ListAffiliateLinkEvents :many
SELECT id, tenant_id, link_id, event_type, referrer_url, ip_hash,
    user_agent_hash, converted_payment_id, commission_amount, created_at
FROM affiliate_link_events
WHERE link_id = @link_id
    AND (@event_type::text IS NULL OR event_type = @event_type)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: IncrementAffiliateLinkClicks :exec
UPDATE user_affiliate_links
SET click_count = click_count + 1
WHERE id = @link_id;

-- name: IncrementAffiliateLinkConversions :exec
UPDATE user_affiliate_links
SET conversion_count = conversion_count + 1,
    total_earned = total_earned + @commission
WHERE id = @link_id;

-- =====================
-- Earnings Aggregate
-- =====================

-- name: GetUserAffiliateEarnings :one
SELECT
    COALESCE(SUM(CASE WHEN event_type = 'click' THEN 1 ELSE 0 END), 0) AS total_clicks,
    COALESCE(SUM(CASE WHEN event_type = 'conversion' THEN 1 ELSE 0 END), 0) AS total_conversions,
    COALESCE(SUM(commission_amount) FILTER (WHERE event_type = 'conversion'), 0) AS total_earned
FROM affiliate_link_events ale
JOIN user_affiliate_links ual ON ual.id = ale.link_id
WHERE ual.user_id = @user_id AND ual.tenant_id = @tenant_id;

-- name: GetUserAffiliateEarningsByProduct :many
SELECT
    ual.product_type,
    ual.product_id,
    COUNT(CASE WHEN ale.event_type = 'click' THEN 1 END) AS clicks,
    COUNT(CASE WHEN ale.event_type = 'conversion' THEN 1 END) AS conversions,
    COALESCE(SUM(ale.commission_amount) FILTER (WHERE ale.event_type = 'conversion'), 0) AS earned
FROM affiliate_link_events ale
JOIN user_affiliate_links ual ON ual.id = ale.link_id
WHERE ual.user_id = @user_id AND ual.tenant_id = @tenant_id
GROUP BY ual.product_type, ual.product_id;

-- name: GetRecentAffiliateEvents :many
SELECT ale.id, ale.event_type, ale.commission_amount, ale.created_at,
    ual.code AS link_code, ual.product_type
FROM affiliate_link_events ale
JOIN user_affiliate_links ual ON ual.id = ale.link_id
WHERE ual.user_id = @user_id AND ual.tenant_id = @tenant_id
ORDER BY ale.created_at DESC
LIMIT @page_limit;

-- =====================
-- Dedup check (for redirect handler)
-- =====================

-- name: CheckAffiliateLinkExists :one
SELECT id, code, destination_url, is_active, product_type, product_id, user_id
FROM user_affiliate_links
WHERE code = @code;
