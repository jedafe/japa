-- name: CreatePlatformEvent :one
INSERT INTO platform_events (event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)

RETURNING id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic, created_at;

-- name: ListPlatformEvents :many
-- Ref: sections/40-platform-analytics-reports.md, ADR-068 §4 (P10)
-- Includes is_synthetic so the admin event list can distinguish
-- persona-originated events from real user activity.
SELECT id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic, created_at
FROM platform_events
WHERE ($1::text = '' OR event_type = $1)
 AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR user_id = $2)
 AND ($3::text = '' OR entity_type = $3)
 AND ($4::timestamptz IS NULL OR created_at >= $4)
 AND ($5::timestamptz IS NULL OR created_at <= $5)
ORDER BY created_at DESC, id DESC
LIMIT $6 OFFSET $7;

-- name: GetEventCountsByType :many
-- Ref: ADR-068 §4 (P10) — real-metric reporting excludes synthetic events
-- (persona-originated activity) so admin dashboards reflect real user engagement.
SELECT event_type, COUNT(*) AS count
FROM platform_events
WHERE is_synthetic = false
 AND ($1::timestamptz IS NULL OR created_at >= $1)
 AND ($2::timestamptz IS NULL OR created_at <= $2)
GROUP BY event_type
ORDER BY count DESC;

-- =====================
-- Materialized Views (refreshed by analytics:refresh_views Asynq task)
-- =====================

-- name: GetDailyContentStats :many
-- Ref: sections/40-platform-analytics-reports.md
-- BUG-ANALYTICS-01 fix: corrected column mapping (reactions from reactions
-- table, comments from comment_count, shares from share_count+reshare_count,
-- unique_viewers from user_content_views distinct count).
SELECT date_trunc('day', c.created_at)::date AS day, c.id AS content_id, c.title AS content_title,
 c.view_count AS views,
 (SELECT COUNT(*)::int FROM reactions r WHERE r.entity_type = 'content' AND r.entity_id = c.id) AS reactions,
 c.comment_count AS comments,
 (c.share_count + c.reshare_count) AS shares,
 (SELECT COUNT(DISTINCT user_id)::int FROM user_content_views ucv WHERE ucv.content_id = c.id AND ucv.user_id IS NOT NULL) AS unique_viewers
FROM contents c
WHERE c.status != 'draft'
 AND ($1::date IS NULL OR c.created_at::date >= $1)
 AND ($2::date IS NULL OR c.created_at::date <= $2)
 AND ($3::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR c.id = $3)
ORDER BY day DESC, views DESC;

-- name: GetTopContentByViews :many
-- BUG-ANALYTICS-02 fix: corrected column mapping (total_reactions from
-- reactions table, total_shares from share_count+reshare_count,
-- total_unique_viewers from user_content_views distinct count).
SELECT c.id AS content_id, c.title AS content_title, c.view_count AS total_views, c.comment_count AS total_comments,
 (SELECT COUNT(*)::int FROM reactions r WHERE r.entity_type = 'content' AND r.entity_id = c.id) AS total_reactions,
 (c.share_count + c.reshare_count) AS total_shares,
 (SELECT COUNT(DISTINCT user_id)::int FROM user_content_views ucv WHERE ucv.content_id = c.id AND ucv.user_id IS NOT NULL) AS total_unique_viewers
FROM contents c
WHERE c.status != 'draft'
 AND c.view_count > 0
 AND ($1::date IS NULL OR c.created_at::date >= $1)
 AND ($2::date IS NULL OR c.created_at::date <= $2)
ORDER BY total_views DESC
LIMIT $3;

-- name: GetUserTrafficSources :many
-- Section 40 user dashboard: traffic sources for a content creator's posts,
-- derived from platform_events.meta->>'referrer'. Falls back to 'Direct'.
-- Excludes synthetic persona activity per ADR-068 §4 (P10).
SELECT
 COALESCE(NULLIF(pe.meta->>'referrer', ''), 'Direct')::text AS referrer,
 COUNT(*)::bigint AS visits
FROM platform_events pe
JOIN contents c ON c.id = pe.entity_id
WHERE c.author_id = $1
 AND pe.event_type = 'content.viewed'
 AND pe.entity_type = 'content'
 AND pe.is_synthetic = false
 AND ($2::timestamptz IS NULL OR pe.created_at >= $2)
 AND ($3::timestamptz IS NULL OR pe.created_at <= $3)
GROUP BY referrer
ORDER BY visits DESC
LIMIT $4;

-- name: GetUserAdPerformance :many
-- Section 40 user dashboard: ad campaign aggregate performance for the
-- authenticated user (ad_campaigns.created_by). Returns 0 for campaigns with
-- no daily analytics rows in the selected range.
SELECT
 ac.id AS campaign_id,
 ac.name AS campaign_name,
 ac.status,
 COALESCE(SUM(aad.impressions), 0)::bigint AS impressions,
 COALESCE(SUM(aad.clicks), 0)::bigint AS clicks,
 COALESCE(SUM(aad.spent), 0)::bigint AS spent,
 CASE
 WHEN COALESCE(SUM(aad.impressions), 0) > 0
 THEN ROUND(100.0 * COALESCE(SUM(aad.clicks), 0) / SUM(aad.impressions), 2)
 ELSE 0
 END::float AS ctr
FROM ad_campaigns ac
LEFT JOIN ad_analytics_daily aad ON aad.campaign_id = ac.id
 AND ($2::date IS NULL OR aad.date >= $2)
 AND ($3::date IS NULL OR aad.date <= $3)
WHERE ac.created_by = $1
GROUP BY ac.id, ac.name, ac.status
ORDER BY impressions DESC
LIMIT $4;

-- name: GetDailySpaceStats :many
SELECT date_trunc('day', s.created_at)::date AS day, s.id AS space_id, s.name AS space_name,
 (SELECT COUNT(*)::bigint FROM space_members sm WHERE sm.space_id = s.id) AS joins,
 (SELECT COUNT(*)::bigint FROM contents c WHERE c.space_id = s.id AND c.status != 'draft') AS posts,
 (SELECT COUNT(DISTINCT sm2.user_id)::bigint FROM space_members sm2 WHERE sm2.space_id = s.id) AS active_users
FROM spaces s
WHERE s.deleted_at IS NULL
 AND ($1::date IS NULL OR s.created_at::date >= $1)
 AND ($2::date IS NULL OR s.created_at::date <= $2)
 AND ($3::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR s.id = $3)
ORDER BY day DESC;

-- name: GetUserEngagementStats :many
-- Section 40 user dashboard: daily engagement computed directly from the
-- operational tables so it works regardless of platform_events population
-- or materialized-view refresh lag. posts_created excludes drafts.
WITH posts AS (
 SELECT date_trunc('day', c.created_at)::date AS day,
 COUNT(*)::bigint AS posts_created
 FROM contents c
 WHERE c.author_id = $3
 AND c.status != 'draft'
 AND ($1::timestamptz IS NULL OR c.created_at >= $1)
 AND ($2::timestamptz IS NULL OR c.created_at <= $2)
 GROUP BY 1
),
reactions AS (
 SELECT date_trunc('day', r.created_at)::date AS day,
 COUNT(*)::bigint AS reactions_given
 FROM reactions r
 WHERE r.user_id = $3
 AND ($1::timestamptz IS NULL OR r.created_at >= $1)
 AND ($2::timestamptz IS NULL OR r.created_at <= $2)
 GROUP BY 1
),
comments_made AS (
 SELECT date_trunc('day', cm.created_at)::date AS day,
 COUNT(*)::bigint AS comments_made
 FROM comments cm
 WHERE cm.author_user_id = $3
 AND cm.deleted_at IS NULL
 AND ($1::timestamptz IS NULL OR cm.created_at >= $1)
 AND ($2::timestamptz IS NULL OR cm.created_at <= $2)
 GROUP BY 1
),
views AS (
 SELECT date_trunc('day', ucv.viewed_at AT TIME ZONE 'UTC')::date AS day,
 COUNT(*)::bigint AS views
 FROM user_content_views ucv
 JOIN contents c ON c.id = ucv.content_id
 WHERE c.author_id = $3
 AND ($1::timestamptz IS NULL OR ucv.viewed_at >= $1)
 AND ($2::timestamptz IS NULL OR ucv.viewed_at <= $2)
 GROUP BY 1
),
days AS (
 SELECT day FROM posts
 UNION
 SELECT day FROM reactions
 UNION
 SELECT day FROM comments_made
 UNION
 SELECT day FROM views
)
SELECT d.day, $3::uuid AS user_id,
 COALESCE(p.posts_created, 0) AS posts_created,
 COALESCE(r.reactions_given, 0) AS reactions_given,
 COALESCE(cm.comments_made, 0) AS comments_made,
 COALESCE(v.views, 0) AS views
FROM days d
LEFT JOIN posts p ON p.day = d.day
LEFT JOIN reactions r ON r.day = d.day
LEFT JOIN comments_made cm ON cm.day = d.day
LEFT JOIN views v ON v.day = d.day
ORDER BY d.day DESC;

-- name: RefreshDailyContentStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_content_stats;

-- name: RefreshDailySpaceStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_space_stats;

-- name: RefreshUserEngagementStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_user_engagement_stats;

-- Section 66 (ADR-084) — Advanced Analytics materialized views.
-- All four are refreshed by the same analytics:refresh_views task (hourly)
-- alongside the §40 base views. Each has a UNIQUE index so CONCURRENTLY
-- is supported. Ref: sections/66-advanced-analytics.md
-- name: RefreshTopContent7dView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_top_content_7d;

-- name: RefreshTopContent30dView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_top_content_30d;

-- name: RefreshTopContent90dView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_top_content_90d;

-- name: RefreshRevenueStreamsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_revenue_streams;

-- =====================
-- Dashboard Aggregates (from real tables)
-- =====================

-- name: GetUserGrowthOverTime :many
-- Fills the full date range with 0-value days so the chart always shows
-- a complete line instead of "No data available" on periods with no signups.
SELECT gs.day::date AS day, COALESCE(u.new_users, 0)::bigint AS new_users
FROM generate_series(
 $1::date,
 COALESCE($2::date, NOW()::date),
 INTERVAL '1 day'
) AS gs(day)
LEFT JOIN (
 SELECT date_trunc('day', created_at)::date AS day, COUNT(*)::bigint AS new_users
 FROM users
 WHERE is_synthetic = false
 AND ($1::date IS NULL OR created_at::date >= $1)
 AND ($2::date IS NULL OR created_at::date <= $2)
 GROUP BY 1
) u ON u.day = gs.day::date
ORDER BY gs.day;

-- =====================
-- Space Analytics (single-query aggregate)
-- =====================

-- name: GetSpaceAnalyticsSummary :one
-- Returns member_count, content_count, view_count, follower_count, and
-- posts_last_30d in a single round trip instead of sequential queries
-- (BUG-020). Each subquery is independent so the planner can parallelise
-- them. member_count counts approved members only (SPACES-006); content
-- counts use the authoritative content_spaces junction (BUG-010).
SELECT
 (SELECT COUNT(*) FROM space_members
 WHERE space_members.space_id = @space_id AND space_members.status = 'approved')::int AS member_count,
 (SELECT COUNT(*) FROM content_spaces cs
 JOIN contents c ON c.id = cs.content_id
 WHERE cs.space_id = @space_id AND c.deleted_at IS NULL AND c.status = 'published')::int AS content_count,
 -- View count reads user_content_views (the table view tracking actually
 -- writes per Section 14) joined through the space's published content;
 -- content_views has no writers and would report a permanent 0.
 (SELECT COUNT(*) FROM user_content_views ucv
 JOIN content_spaces cs ON cs.content_id = ucv.content_id
 JOIN contents c ON c.id = cs.content_id
 WHERE cs.space_id = @space_id AND c.deleted_at IS NULL AND c.status = 'published')::int AS view_count,
 (SELECT COUNT(*) FROM follows f
 WHERE f.entity_type = 'space' AND f.entity_id = @space_id)::int AS follower_count,
 (SELECT COUNT(*) FROM content_spaces cs
 JOIN contents c ON c.id = cs.content_id
 WHERE cs.space_id = @space_id AND c.deleted_at IS NULL AND c.status = 'published'
 AND c.created_at >= now() - interval '30 days')::int AS posts_last_30d;

-- name: GetDailyActiveUsers :many
SELECT date_trunc('day', last_login_at)::date AS day, COUNT(*) AS dau
FROM users
WHERE is_synthetic = false
 AND last_login_at IS NOT NULL
 AND ($1::timestamptz IS NULL OR last_login_at >= $1)
 AND ($2::timestamptz IS NULL OR last_login_at <= $2)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetMonthlyActiveUsers :many
SELECT date_trunc('month', last_login_at)::date AS month, COUNT(*) AS mau
FROM users
WHERE is_synthetic = false
 AND last_login_at IS NOT NULL
 AND ($1::timestamptz IS NULL OR last_login_at >= $1)
 AND ($2::timestamptz IS NULL OR last_login_at <= $2)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetContentVolumeOverTime :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(*) AS content_count
FROM contents
WHERE status != 'draft'
 AND ($1::timestamptz IS NULL OR created_at >= $1)
 AND ($2::timestamptz IS NULL OR created_at <= $2)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetTrafficSources :many
-- Traffic sources are derived from content view_count aggregated by the
-- space that owns the content. Content without a space is labelled "Direct".
SELECT COALESCE(s.name, 'Direct') AS referrer, COALESCE(SUM(c.view_count), 0)::bigint AS visits
FROM contents c
LEFT JOIN spaces s ON s.id = c.space_id
WHERE c.status != 'draft'
 AND c.view_count > 0
 AND ($1::timestamptz IS NULL OR c.created_at >= $1)
 AND ($2::timestamptz IS NULL OR c.created_at <= $2)
GROUP BY 1
ORDER BY visits DESC
LIMIT $3;

-- name: GetTotalUsers :one
-- Real user accounts only — synthetic video channels (users.is_synthetic,
-- migration 000633) are excluded so dashboard totals reflect real signups.
SELECT COUNT(*)::integer FROM users WHERE is_synthetic = false;

-- name: GetActiveUsers :one
SELECT COUNT(*)::integer FROM users WHERE is_synthetic = false AND last_login_at IS NOT NULL AND last_login_at >= NOW() - INTERVAL '30 days';

-- name: GetNewUsersToday :one
SELECT COUNT(*)::integer FROM users WHERE is_synthetic = false AND created_at::date = CURRENT_DATE;

-- =====================
-- Security Scan Results
-- =====================

-- name: CreateSecurityScanResult :one
-- Upsert by (scan_type) on the MOST RECENT row regardless of
-- resolved state. This prevents the control panel security page from
-- showing the same finding repeated on every scan run, AND prevents a
-- resolved finding from reappearing as a new row when the condition is
-- re-detected by a later scan.
--
-- Behavior:
-- * No existing row of this scan_type -> INSERT a new finding.
-- * Existing UNRESOLVED row -> refresh severity/finding/meta
-- and bump created_at (so the
-- "last checked" time is current).
-- * Existing RESOLVED row -> refresh severity/finding/meta
-- in place but KEEP resolved_at
-- set and do NOT bump created_at,
-- so the resolved finding does not
-- reappear as a new active warning.
WITH target AS (
 SELECT s0.id, (s0.resolved_at IS NOT NULL) AS is_resolved
 FROM security_scan_results s0
 WHERE s0.scan_type = $1
 ORDER BY s0.created_at DESC, s0.id DESC
 LIMIT 1
),
updated AS (
 UPDATE security_scan_results AS s
 SET severity = $2,
 finding = $3,
 meta = $4,
 created_at = CASE WHEN target.is_resolved THEN s.created_at ELSE now() END
 FROM target
 WHERE s.id = target.id
 RETURNING s.id, s.scan_type, s.severity, s.finding, s.meta, s.resolved_at, s.created_at
),
inserted AS (
 INSERT INTO security_scan_results (scan_type, severity, finding, meta)
 SELECT $1, $2, $3, $4
 WHERE NOT EXISTS (SELECT 1 FROM updated)
 RETURNING id, scan_type, severity, finding, meta, resolved_at, created_at
)
SELECT u.id, u.scan_type, u.severity, u.finding, u.meta, u.resolved_at, u.created_at FROM updated u
UNION ALL
SELECT i.id, i.scan_type, i.severity, i.finding, i.meta, i.resolved_at, i.created_at FROM inserted i;

-- name: ListSecurityScanResults :many
SELECT id, scan_type, severity, finding, meta, resolved_at, created_at
FROM security_scan_results
WHERE ($1::text = '' OR severity = $1)
 AND ($2::bool = false OR resolved_at IS NULL)
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4;

-- name: ResolveSecurityScanResult :exec
UPDATE security_scan_results
SET resolved_at = now()
WHERE id = $1;

-- name: DeleteSecurityScanResult :exec
-- Deletes a single scan finding. Used by the control panel to clear
-- resolved findings from the audit view. Platform-scoped for safety.
DELETE FROM security_scan_results
WHERE id = $1;

-- name: GetUnresolvedCriticalFindings :many
SELECT id, scan_type, severity, finding, meta, created_at
FROM security_scan_results
WHERE resolved_at IS NULL
 AND severity = 'critical'
ORDER BY created_at DESC;

-- =====================
-- Revenue Analytics (P5.6)
-- =====================

-- name: GetRevenueSummary :one
SELECT
 COALESCE(SUM(p.amount), 0)::bigint AS total_revenue,
 COALESCE(SUM(CASE WHEN p.created_at >= date_trunc('month', NOW()) THEN p.amount ELSE 0 END), 0)::bigint AS monthly_revenue,
 (SELECT COUNT(*)::integer FROM subscriptions s WHERE s.status = 'active') AS active_subscriptions
FROM payments p
WHERE p.status = 'succeeded';

-- name: GetRevenueByPeriod :many
SELECT
 to_char(date_trunc('month', p.created_at), 'YYYY-MM') AS period,
 COALESCE(SUM(p.amount), 0)::bigint AS revenue,
 COUNT(*)::integer AS transactions
FROM payments p
WHERE p.status = 'succeeded'
 AND ($1::timestamptz IS NULL OR p.created_at >= $1)
 AND ($2::timestamptz IS NULL OR p.created_at <= $2)
GROUP BY 1
ORDER BY 1 DESC;

-- =====================
-- Dashboard Moderation / AI Overview Counts
-- =====================

-- name: CountPendingReports :one
SELECT COUNT(*)::int FROM content_reports
WHERE status = 'pending';

-- name: CountOpenTickets :one
SELECT COUNT(*)::int FROM tickets
WHERE status NOT IN ('resolved', 'closed', 'spam');

-- name: CountSpamQueue :one
SELECT
 (SELECT COUNT(*)::int FROM media m
 WHERE m.deleted_at IS NULL
 AND (m.flagged = true OR m.quarantine_reason IS NOT NULL))
 +
 (SELECT COUNT(*)::int FROM contents c
 WHERE c.deleted_at IS NULL
 AND c.auto_hidden = true) AS count;

-- name: CountGenerationsToday :one
SELECT COUNT(*)::int FROM studio_jobs
WHERE created_at >= date_trunc('day', NOW());

-- name: CountPendingReviews :one
SELECT COUNT(*)::int FROM content_reviews
WHERE status = 'pending' AND deleted_at IS NULL;

-- =====================
-- Content Ownership (for analytics access control)
-- =====================

-- name: GetContentAuthorID :one
-- Ref: api-routes-catalog.md — GET /content/{id}/analytics is owner+admin only.
-- Returns the author_id so the handler can verify the caller is the owner.
SELECT author_id FROM contents
WHERE id = $1 AND deleted_at IS NULL;

-- =====================
-- Space Analytics (single-query aggregate)
-- =====================
