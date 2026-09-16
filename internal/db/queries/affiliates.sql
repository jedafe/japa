-- name: CreateAffiliateMember :one
INSERT INTO affiliate_program_members (user_id, status) VALUES (@user_id, @status)

ON CONFLICT (user_id) DO UPDATE
 SET status = EXCLUDED.status
RETURNING user_id, enrolled_at, status, total_clicks,
 total_conversions, total_earned_credits, payout_method, notes;

-- name: GetAffiliateMember :one
SELECT user_id, enrolled_at, status, total_clicks,
 total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE user_id = @user_id;

-- name: GetAffiliateMemberByUserID :one
SELECT user_id, enrolled_at, status, total_clicks,
 total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE user_id = @user_id;

-- name: ListAffiliateMembers :many
SELECT user_id, enrolled_at, status, total_clicks,
 total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members

ORDER BY enrolled_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAffiliateMemberStatus :one
UPDATE affiliate_program_members
SET status = @status, notes = sqlc.narg(notes)
WHERE user_id = @user_id
RETURNING user_id, enrolled_at, status, total_clicks,
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

-- name: CountAffiliateMembers :one
SELECT COUNT(*) AS count
FROM affiliate_program_members
;

-- =====================
-- User Affiliate Links CRUD
-- =====================

-- name: CreateAffiliateLink :one
INSERT INTO user_affiliate_links (user_id, product_type, product_id, code, description, destination_url) VALUES (@user_id, @product_type, sqlc.narg(product_id), @code, sqlc.narg(description), @destination_url)
RETURNING id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at;

-- name: GetAffiliateLinkByID :one
SELECT id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE id = @id;

-- name: GetAffiliateLinkByIDOnly :one
SELECT id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE id = @id;

-- name: GetAffiliateLinkByCode :one
SELECT id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE code = @code AND is_active = true;

-- name: ListAffiliateLinksByUser :many
SELECT id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links
WHERE user_id = @user_id
 AND (@is_active::bool IS NULL OR is_active = @is_active)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAllAffiliateLinks :many
SELECT id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at
FROM user_affiliate_links

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAffiliateLink :one
UPDATE user_affiliate_links
SET description = sqlc.narg(description), is_active = @is_active
WHERE id = @id AND user_id = @user_id
RETURNING id, user_id, product_type, product_id, code, description,
 destination_url, click_count, conversion_count, total_earned, is_active, created_at;

-- name: DeactivateAffiliateLink :exec
UPDATE user_affiliate_links
SET is_active = false
WHERE id = @id AND user_id = @user_id;

-- name: CountActiveLinksByUser :one
SELECT COUNT(*) AS count
FROM user_affiliate_links
WHERE user_id = @user_id AND is_active = true;

-- name: DeactivateLinksByMemberStatus :exec
UPDATE user_affiliate_links
SET is_active = false
WHERE user_id = @user_id AND is_active = true;

-- name: DeactivateAffiliateLinkAdmin :exec
-- Admin-scoped link deactivation (Section 56 admin management). Deactivates
-- by id + the id without an ownership check (the admin has
-- affiliate.admin.manage). Soft-delete only — the row is retained for audit.
UPDATE user_affiliate_links
SET is_active = false
WHERE id = @id;

-- name: ListAffiliateMembersFiltered :many
-- Admin list with optional status filter (Section 56 / catalog
-- ?status=active|inactive). Platform-scoped. NULL status = no filter.
SELECT user_id, enrolled_at, status, total_clicks,
 total_conversions, total_earned_credits, payout_method, notes
FROM affiliate_program_members
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY enrolled_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAllAffiliateLinksFiltered :many
-- Admin list with optional user_id / product_type / is_active filters
-- (Section 56 / catalog ?user_id&product_type&status=active|inactive).
-- Platform-scoped. Joins users for the admin UI's username column. NULL = no
-- filter on that dimension.
SELECT ual.id, ual.user_id, ual.product_type, ual.product_id,
 ual.code, ual.description, ual.destination_url, ual.click_count,
 ual.conversion_count, ual.total_earned, ual.is_active, ual.created_at,
 u.username
FROM user_affiliate_links ual
LEFT JOIN users u ON u.id = ual.user_id
WHERE (sqlc.narg(user_id)::uuid IS NULL OR ual.user_id = sqlc.narg(user_id))
 AND (sqlc.narg(product_type)::text IS NULL OR ual.product_type = sqlc.narg(product_type))
 AND (sqlc.narg(is_active)::bool IS NULL OR ual.is_active = sqlc.narg(is_active))
ORDER BY ual.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Affiliate Link Events
-- =====================

-- name: CreateAffiliateLinkEvent :one
INSERT INTO affiliate_link_events (link_id, event_type, referrer_url, ip_hash, user_agent_hash, converted_payment_id, commission_amount) VALUES (@link_id, @event_type, sqlc.narg(referrer_url), sqlc.narg(ip_hash), sqlc.narg(user_agent_hash), sqlc.narg(converted_payment_id), sqlc.narg(commission_amount))
RETURNING id, link_id, event_type, referrer_url, ip_hash,
 user_agent_hash, converted_payment_id, commission_amount, created_at;

-- name: ListAffiliateLinkEvents :many
SELECT id, link_id, event_type, referrer_url, ip_hash,
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
WHERE ual.user_id = @user_id;

-- name: GetUserAffiliateEarningsByProduct :many
SELECT
 ual.product_type,
 ual.product_id,
 COUNT(CASE WHEN ale.event_type = 'click' THEN 1 END) AS clicks,
 COUNT(CASE WHEN ale.event_type = 'conversion' THEN 1 END) AS conversions,
 COALESCE(SUM(ale.commission_amount) FILTER (WHERE ale.event_type = 'conversion'), 0) AS earned
FROM affiliate_link_events ale
JOIN user_affiliate_links ual ON ual.id = ale.link_id
WHERE ual.user_id = @user_id
GROUP BY ual.product_type, ual.product_id;

-- name: GetRecentAffiliateEvents :many
SELECT ale.id, ale.event_type, ale.commission_amount, ale.created_at,
 ual.code AS link_code, ual.product_type
FROM affiliate_link_events ale
JOIN user_affiliate_links ual ON ual.id = ale.link_id
WHERE ual.user_id = @user_id
ORDER BY ale.created_at DESC
LIMIT @page_limit;

-- =====================
-- Program-wide Stats (admin overview — Section 56)
-- =====================

-- name: GetAffiliateProgramStats :one
-- Returns program-wide totals for the admin overview endpoint
-- GET /admin/affiliate-program (Section 56 Admin management).
-- Platform-scoped: only counts members/links/events for the given platform.
SELECT
 (SELECT COUNT(*) FROM affiliate_program_members apm ) AS total_members,
 (SELECT COUNT(*) FROM affiliate_program_members apm WHERE apm.status = 'active') AS active_members,
 (SELECT COUNT(*) FROM user_affiliate_links ual2 ) AS total_links,
 COALESCE(SUM(ale.click_count), 0) AS total_clicks,
 COALESCE(SUM(ale.conversion_count), 0) AS total_conversions,
 COALESCE(SUM(ale.total_earned), 0) AS total_commissions
FROM user_affiliate_links ale
;

-- =====================
-- Dedup check (for redirect handler)
-- =====================

-- =====================
-- External Affiliate Commissions (Section 55 — Author Revenue Sharing)
-- =====================

-- name: GetAffiliateContentCommissionForCredit :one
-- AUDIT-BE-FIN-001: replaces raw SQL read in HandleAffiliateCreditAuthorCommission.
-- Fetches the commission row for crediting; returns only the fields needed.
SELECT id, author_user_id, author_share, credited
FROM affiliate_content_commissions
WHERE id = @id;

-- name: MarkAffiliateCommissionCredited :execrows
-- AUDIT-BE-FIN-001: replaces raw SQL UPDATE of the commission row.
-- Marks the commission as credited; paired with the wallet_ledger INSERT
-- inside the same transaction (idempotent on commission row ID).
UPDATE affiliate_content_commissions
SET credited = true, credited_at = now()
WHERE id = @id AND credited = false;

-- =====================
-- Record external affiliate commission (Section 55 — Author Revenue Sharing)
-- =====================

-- name: InsertAffiliateContentCommission :one
-- Records an external affiliate commission earned from a content's affiliate link.
-- The author_share is pre-computed by the caller (external_commission * author_commission_pct / 100).
-- After insert, the caller enqueues the affiliate:credit_author_commission task to
-- credit the author_share to the author's wallet via the immutable credits ledger.
INSERT INTO affiliate_content_commissions (content_id, provider_id, author_user_id, external_commission, author_share, author_commission_pct, event_description) VALUES (@content_id, @provider_id, @author_user_id, @external_commission, @author_share, @author_commission_pct, sqlc.narg(event_description))
RETURNING id, content_id, provider_id, author_user_id,
 external_commission, author_share, author_commission_pct, event_description,
 credited, credited_at, created_at;

-- name: GetActiveAffiliateLinkByUser :one
SELECT code FROM user_affiliate_links
WHERE user_id = @user_id AND is_active = true
LIMIT 1;

-- name: GetAffiliateCommissionInfo :one
-- Resolves the affiliate member + commission rate for a link code, reading
-- the rate from system_settings (affiliate.commission.*) per Section 56 —
-- the single source of truth. Falls back to the catalog defaults (15/20/10)
-- when a platform has not configured the rate.
--
-- V1 behavior (BUG-008): pro_subscription always maps to the pro_monthly
-- rate. The pro_annual rate is configurable but unused in V1 — there is no
-- product_type='pro_subscription_annual' distinction. Annual vs monthly
-- distinction is handled at the payment-purpose layer (pro_membership
-- covers both billing cycles). This is accepted V1 behavior; V2 may
-- introduce a separate annual product type if needed.
SELECT apm.user_id, ual.product_type,
 (CASE ual.product_type
 WHEN 'course' THEN COALESCE(
 (SELECT value::numeric FROM system_settings WHERE key = 'affiliate.commission.course'),
 10)
 WHEN 'pro_subscription' THEN COALESCE(
 (SELECT value::numeric FROM system_settings WHERE key = 'affiliate.commission.pro_monthly'),
 15)
 ELSE COALESCE(
 (SELECT value::numeric FROM system_settings WHERE key = 'affiliate.commission.pro_annual'),
 20)
 END)::float8 AS commission_rate
FROM affiliate_program_members apm
JOIN user_affiliate_links ual ON ual.user_id = apm.user_id
WHERE ual.code = @code AND apm.status = 'active' AND ual.is_active = true;

-- name: DeactivateLinksForSuspendedMembers :exec
UPDATE user_affiliate_links
SET is_active = false
WHERE is_active = true
 AND user_id IN (
 SELECT user_id FROM affiliate_program_members
 WHERE status IN ('suspended', 'banned')
 );

-- =====================
-- Affiliate Enhancement Queries (Section 55 — BE-LOW-002)
-- Replaces raw SQL in coupons/handlers.go with sqlc-generated queries.
-- =====================

-- name: ListAffiliateScanResults :many
-- BE-LOW-002: replaces raw SQL in ListAffiliateScanResults handler.
-- Optional domain and action_taken filters via sqlc.narg().
SELECT id, content_id, url, domain, detection_method, action_taken, created_at
FROM content_affiliate_scan_results
WHERE (sqlc.narg(domain)::text IS NULL OR domain = sqlc.narg(domain))
 AND (sqlc.narg(action_taken)::text IS NULL OR action_taken = sqlc.narg(action_taken))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ReviewAffiliateScanResult :execrows
-- BE-LOW-002: replaces raw SQL UPDATE in ReviewAffiliateScanResult handler.
-- Returns rows affected so the handler can distinguish 404 from success.
UPDATE content_affiliate_scan_results
SET action_taken = @action_taken, reviewed_by = sqlc.narg(reviewed_by),
 reviewed_at = now(), notes = sqlc.narg(notes)
WHERE id = @id;

-- name: ListAffiliateCommissions :many
-- BE-LOW-002: replaces raw SQL in ListAffiliateCommissions handler.
-- Optional credited filter via sqlc.narg(). Joins provider name and author username.
SELECT ac.id, ac.content_id, ac.author_user_id, ac.external_commission,
 ac.author_share, ac.author_commission_pct, ac.event_description,
 ac.credited, ac.created_at,
 ap.name AS provider_name, u.username AS author_username
FROM affiliate_content_commissions ac
LEFT JOIN affiliate_providers ap ON ap.id = ac.provider_id
LEFT JOIN users u ON u.id = ac.author_user_id
WHERE (sqlc.narg(credited)::bool IS NULL OR ac.credited = sqlc.narg(credited))
ORDER BY ac.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateProviderAuthorCommission :execrows
-- BE-LOW-002: replaces raw SQL UPDATE in UpdateProviderAuthorCommission handler.
-- Returns rows affected so the handler can distinguish 404 from success.
UPDATE affiliate_providers
SET author_commission_pct = @author_commission_pct, updated_at = now()
WHERE id = @id;
