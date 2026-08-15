-- Platform Analytics & Site Reports (Section 40)
-- Dashboard queries using real database tables (users, contents, spaces, space_members).
-- platform_events and materialized views are kept for the events list and future
-- event-tracking pipeline; dashboard aggregates bypass them so they work immediately.

-- =====================
-- Platform Events (write-optimised, future use)
-- =====================

-- name: CreatePlatformEvent :one
INSERT INTO platform_events (tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic, created_at;

-- name: ListPlatformEvents :many
-- Ref: sections/40-platform-analytics-reports.md, ADR-068 §4 (P10)
-- Includes is_synthetic so the admin event list can distinguish
-- persona-originated events from real user activity.
SELECT id, tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, is_synthetic, created_at
FROM platform_events
WHERE tenant_id = $1
  AND ($2::text = '' OR event_type = $2)
  AND ($3::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR user_id = $3)
  AND ($4::text = '' OR entity_type = $4)
  AND ($5::timestamptz IS NULL OR created_at >= $5)
  AND ($6::timestamptz IS NULL OR created_at <= $6)
ORDER BY created_at DESC, id DESC
LIMIT $7 OFFSET $8;

-- name: GetEventCountsByType :many
-- Ref: ADR-068 §4 (P10) — real-metric reporting excludes synthetic events
-- (persona-originated activity) so admin dashboards reflect real user engagement.
SELECT event_type, COUNT(*) AS count
FROM platform_events
WHERE tenant_id = $1
  AND is_synthetic = false
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
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
SELECT date_trunc('day', c.created_at)::date AS day, c.tenant_id, c.id AS content_id, c.title AS content_title,
       c.view_count AS views,
       (SELECT COUNT(*)::int FROM reactions r WHERE r.tenant_id = c.tenant_id AND r.entity_type = 'content' AND r.entity_id = c.id) AS reactions,
       c.comment_count AS comments,
       (c.share_count + c.reshare_count) AS shares,
       (SELECT COUNT(DISTINCT user_id)::int FROM user_content_views ucv WHERE ucv.content_id = c.id AND ucv.user_id IS NOT NULL) AS unique_viewers
FROM contents c
WHERE c.tenant_id = $1
  AND c.status != 'draft'
  AND ($2::date IS NULL OR c.created_at::date >= $2)
  AND ($3::date IS NULL OR c.created_at::date <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR c.id = $4)
ORDER BY day DESC, views DESC;

-- name: GetTopContentByViews :many
-- BUG-ANALYTICS-02 fix: corrected column mapping (total_reactions from
-- reactions table, total_shares from share_count+reshare_count,
-- total_unique_viewers from user_content_views distinct count).
SELECT c.id AS content_id, c.title AS content_title, c.view_count AS total_views, c.comment_count AS total_comments,
       (SELECT COUNT(*)::int FROM reactions r WHERE r.tenant_id = c.tenant_id AND r.entity_type = 'content' AND r.entity_id = c.id) AS total_reactions,
       (c.share_count + c.reshare_count) AS total_shares,
       (SELECT COUNT(DISTINCT user_id)::int FROM user_content_views ucv WHERE ucv.content_id = c.id AND ucv.user_id IS NOT NULL) AS total_unique_viewers
FROM contents c
WHERE c.tenant_id = $1
  AND c.status != 'draft'
  AND c.view_count > 0
  AND ($2::date IS NULL OR c.created_at::date >= $2)
  AND ($3::date IS NULL OR c.created_at::date <= $3)
ORDER BY total_views DESC
LIMIT $4;

-- name: GetDailySpaceStats :many
SELECT date_trunc('day', s.created_at)::date AS day, s.tenant_id, s.id AS space_id, s.name AS space_name,
       (SELECT COUNT(*)::bigint FROM space_members sm WHERE sm.space_id = s.id) AS joins,
       (SELECT COUNT(*)::bigint FROM contents c WHERE c.space_id = s.id AND c.status != 'draft') AS posts,
       (SELECT COUNT(DISTINCT sm2.user_id)::bigint FROM space_members sm2 WHERE sm2.space_id = s.id) AS active_users
FROM spaces s
WHERE s.tenant_id = $1
  AND s.deleted_at IS NULL
  AND ($2::date IS NULL OR s.created_at::date >= $2)
  AND ($3::date IS NULL OR s.created_at::date <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR s.id = $4)
ORDER BY day DESC;

-- name: GetUserEngagementStats :many
SELECT day, tenant_id, user_id, posts_created, reactions_given, comments_made, views
FROM mv_user_engagement_stats
WHERE tenant_id = $1
  AND ($2::date IS NULL OR day >= $2)
  AND ($3::date IS NULL OR day <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR user_id = $4)
ORDER BY day DESC;

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
  $2::date,
  COALESCE($3::date, NOW()::date),
  INTERVAL '1 day'
) AS gs(day)
LEFT JOIN (
  SELECT date_trunc('day', created_at)::date AS day, COUNT(*)::bigint AS new_users
  FROM users
  WHERE tenant_id = $1
    AND ($2::date IS NULL OR created_at::date >= $2)
    AND ($3::date IS NULL OR created_at::date <= $3)
  GROUP BY 1
) u ON u.day = gs.day::date
ORDER BY gs.day;

-- =====================
-- Space Analytics (single-query aggregate)
-- =====================

-- name: GetSpaceAnalyticsSummary :one
-- Returns member_count, content_count, and view_count in a single round
-- trip instead of three sequential queries (BUG-020). Each subquery is
-- independent so the planner can parallelise them.
SELECT
  (SELECT COUNT(*) FROM space_members
     WHERE space_members.space_id = @space_id AND space_members.tenant_id = @tenant_id)::int AS member_count,
  (SELECT COUNT(*) FROM contents
     WHERE contents.space_id = @space_id AND contents.tenant_id = @tenant_id AND contents.status = 'published')::int AS content_count,
  (SELECT COUNT(*) FROM content_views
     WHERE content_views.space_id = @space_id AND content_views.tenant_id = @tenant_id)::int AS view_count;

-- name: GetDailyActiveUsers :many
SELECT date_trunc('day', last_login_at)::date AS day, COUNT(*) AS dau
FROM users
WHERE tenant_id = $1
  AND last_login_at IS NOT NULL
  AND ($2::timestamptz IS NULL OR last_login_at >= $2)
  AND ($3::timestamptz IS NULL OR last_login_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetMonthlyActiveUsers :many
SELECT date_trunc('month', last_login_at)::date AS month, COUNT(*) AS mau
FROM users
WHERE tenant_id = $1
  AND last_login_at IS NOT NULL
  AND ($2::timestamptz IS NULL OR last_login_at >= $2)
  AND ($3::timestamptz IS NULL OR last_login_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetContentVolumeOverTime :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(*) AS content_count
FROM contents
WHERE tenant_id = $1
  AND status != 'draft'
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetTrafficSources :many
-- Traffic sources are derived from content view_count aggregated by the
-- space that owns the content. Content without a space is labelled "Direct".
SELECT COALESCE(s.name, 'Direct') AS referrer, COALESCE(SUM(c.view_count), 0)::bigint AS visits
FROM contents c
LEFT JOIN spaces s ON s.id = c.space_id AND s.tenant_id = c.tenant_id
WHERE c.tenant_id = $1
  AND c.status != 'draft'
  AND c.view_count > 0
  AND ($2::timestamptz IS NULL OR c.created_at >= $2)
  AND ($3::timestamptz IS NULL OR c.created_at <= $3)
GROUP BY 1
ORDER BY visits DESC
LIMIT $4;

-- name: GetTotalUsers :one
SELECT COUNT(*)::integer FROM users WHERE tenant_id = $1;

-- name: GetActiveUsers :one
SELECT COUNT(*)::integer FROM users WHERE tenant_id = $1 AND last_login_at IS NOT NULL AND last_login_at >= NOW() - INTERVAL '30 days';

-- name: GetNewUsersToday :one
SELECT COUNT(*)::integer FROM users WHERE tenant_id = $1 AND created_at::date = CURRENT_DATE;

-- =====================
-- Security Scan Results
-- =====================

-- name: CreateSecurityScanResult :one
-- Upsert by (tenant_id, scan_type) on the MOST RECENT row regardless of
-- resolved state. This prevents the control panel security page from
-- showing the same finding repeated on every scan run, AND prevents a
-- resolved finding from reappearing as a new row when the condition is
-- re-detected by a later scan.
--
-- Behavior:
--   * No existing row of this scan_type  -> INSERT a new finding.
--   * Existing UNRESOLVED row            -> refresh severity/finding/meta
--                                           and bump created_at (so the
--                                           "last checked" time is current).
--   * Existing RESOLVED row              -> refresh severity/finding/meta
--                                           in place but KEEP resolved_at
--                                           set and do NOT bump created_at,
--                                           so the resolved finding does not
--                                           reappear as a new active warning.
WITH target AS (
    SELECT s0.id, (s0.resolved_at IS NOT NULL) AS is_resolved
    FROM security_scan_results s0
    WHERE s0.tenant_id = $1
      AND s0.scan_type = $2
    ORDER BY s0.created_at DESC, s0.id DESC
    LIMIT 1
),
updated AS (
    UPDATE security_scan_results AS s
    SET severity   = $3,
        finding    = $4,
        meta       = $5,
        created_at = CASE WHEN target.is_resolved THEN s.created_at ELSE now() END
    FROM target
    WHERE s.id = target.id
    RETURNING s.id, s.tenant_id, s.scan_type, s.severity, s.finding, s.meta, s.resolved_at, s.created_at
),
inserted AS (
    INSERT INTO security_scan_results (tenant_id, scan_type, severity, finding, meta)
    SELECT $1, $2, $3, $4, $5
    WHERE NOT EXISTS (SELECT 1 FROM updated)
    RETURNING id, tenant_id, scan_type, severity, finding, meta, resolved_at, created_at
)
SELECT u.id, u.tenant_id, u.scan_type, u.severity, u.finding, u.meta, u.resolved_at, u.created_at FROM updated u
UNION ALL
SELECT i.id, i.tenant_id, i.scan_type, i.severity, i.finding, i.meta, i.resolved_at, i.created_at FROM inserted i;

-- name: ListSecurityScanResults :many
SELECT id, tenant_id, scan_type, severity, finding, meta, resolved_at, created_at
FROM security_scan_results
WHERE tenant_id = $1
  AND ($2::text = '' OR severity = $2)
  AND ($3::bool = false OR resolved_at IS NULL)
ORDER BY created_at DESC, id DESC
LIMIT $4 OFFSET $5;

-- name: ResolveSecurityScanResult :exec
UPDATE security_scan_results
SET resolved_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteSecurityScanResult :exec
-- Deletes a single scan finding. Used by the control panel to clear
-- resolved findings from the audit view. Tenant-scoped for safety.
DELETE FROM security_scan_results
WHERE id = $1 AND tenant_id = $2;

-- name: GetUnresolvedCriticalFindings :many
SELECT id, tenant_id, scan_type, severity, finding, meta, created_at
FROM security_scan_results
WHERE tenant_id = $1
  AND resolved_at IS NULL
  AND severity = 'critical'
ORDER BY created_at DESC;

-- =====================
-- Revenue Analytics (P5.6)
-- =====================

-- name: GetRevenueSummary :one
SELECT
  COALESCE(SUM(p.amount), 0)::bigint AS total_revenue,
  COALESCE(SUM(CASE WHEN p.created_at >= date_trunc('month', NOW()) THEN p.amount ELSE 0 END), 0)::bigint AS monthly_revenue,
  (SELECT COUNT(*)::integer FROM subscriptions s WHERE s.tenant_id = $1 AND s.status = 'active') AS active_subscriptions
FROM payments p
WHERE p.tenant_id = $1
  AND p.status = 'succeeded';

-- name: GetRevenueByPeriod :many
SELECT
  to_char(date_trunc('month', p.created_at), 'YYYY-MM') AS period,
  COALESCE(SUM(p.amount), 0)::bigint AS revenue,
  COUNT(*)::integer AS transactions
FROM payments p
WHERE p.tenant_id = $1
  AND p.status = 'succeeded'
  AND ($2::timestamptz IS NULL OR p.created_at >= $2)
  AND ($3::timestamptz IS NULL OR p.created_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- =====================
-- Dashboard Moderation / AI Overview Counts
-- =====================

-- name: CountPendingReports :one
SELECT COUNT(*)::int FROM content_reports
WHERE tenant_id = $1 AND status = 'pending';

-- name: CountOpenTickets :one
SELECT COUNT(*)::int FROM tickets
WHERE tenant_id = $1 AND status NOT IN ('resolved', 'closed', 'spam');

-- name: CountSpamQueue :one
SELECT
  (SELECT COUNT(*)::int FROM media m
   WHERE m.tenant_id = $1 AND m.deleted_at IS NULL
     AND (m.flagged = true OR m.quarantine_reason IS NOT NULL))
  +
  (SELECT COUNT(*)::int FROM contents c
   WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
     AND c.auto_hidden = true) AS count;

-- name: CountGenerationsToday :one
SELECT COUNT(*)::int FROM studio_jobs
WHERE tenant_id = $1 AND created_at >= date_trunc('day', NOW());

-- name: CountPendingReviews :one
SELECT COUNT(*)::int FROM content_reviews
WHERE tenant_id = $1 AND status = 'pending' AND deleted_at IS NULL;

-- =====================
-- Content Ownership (for analytics access control)
-- =====================

-- name: GetContentAuthorID :one
-- Ref: api-routes-catalog.md — GET /content/{id}/analytics is owner+admin only.
-- Returns the author_id so the handler can verify the caller is the owner.
SELECT author_id FROM contents
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- =====================
-- Space Analytics (single-query aggregate)
-- =====================
