-- Platform Analytics & Site Reports (Section 40)
-- Dashboard queries using real database tables (users, contents, spaces, space_members).
-- platform_events and materialized views are kept for the events list and future
-- event-tracking pipeline; dashboard aggregates bypass them so they work immediately.

-- =====================
-- Platform Events (write-optimised, future use)
-- =====================

-- name: CreatePlatformEvent :one
INSERT INTO platform_events (tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, created_at;

-- name: ListPlatformEvents :many
SELECT id, tenant_id, event_type, user_id, entity_type, entity_id, meta, country_code, session_id, created_at
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
SELECT event_type, COUNT(*) AS count
FROM platform_events
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY event_type
ORDER BY count DESC;

-- =====================
-- Materialized Views (refreshed by analytics:refresh_views Asynq task)
-- =====================

-- name: GetDailyContentStats :many
SELECT date_trunc('day', c.created_at)::date AS day, c.tenant_id, c.id AS content_id, c.title AS content_title,
       c.view_count AS views, c.comment_count AS reactions, c.share_count AS comments, c.view_count AS unique_viewers
FROM contents c
WHERE c.tenant_id = $1
  AND c.status != 'draft'
  AND ($2::date IS NULL OR c.created_at::date >= $2)
  AND ($3::date IS NULL OR c.created_at::date <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR c.id = $4)
ORDER BY day DESC, views DESC;

-- name: GetTopContentByViews :many
SELECT id AS content_id, title AS content_title, view_count AS total_views, comment_count AS total_comments, share_count AS total_reactions, view_count AS total_unique_viewers
FROM contents
WHERE tenant_id = $1
  AND status != 'draft'
  AND view_count > 0
  AND ($2::date IS NULL OR created_at::date >= $2)
  AND ($3::date IS NULL OR created_at::date <= $3)
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

-- =====================
-- Dashboard Aggregates (from real tables)
-- =====================

-- name: GetUserGrowthOverTime :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(*) AS new_users
FROM users
WHERE tenant_id = $1
  AND ($2::date IS NULL OR created_at::date >= $2)
  AND ($3::date IS NULL OR created_at::date <= $3)
GROUP BY 1
ORDER BY 1 DESC;

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
SELECT field_values->>'referrer' AS referrer, COUNT(*) AS visits
FROM contents
WHERE tenant_id = $1
  AND status != 'draft'
  AND field_values->>'referrer' IS NOT NULL
  AND field_values->>'referrer' != ''
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY field_values->>'referrer'
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
INSERT INTO security_scan_results (tenant_id, scan_type, severity, finding, meta)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, scan_type, severity, finding, meta, resolved_at, created_at;

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
-- Space Analytics (single-query aggregate)
-- =====================
