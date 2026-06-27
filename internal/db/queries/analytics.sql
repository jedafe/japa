-- Platform Analytics & Site Reports (Section 40)
-- Platform events, materialized view queries, and security scan results.

-- =====================
-- Platform Events
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
-- Materialized View: Daily Content Stats
-- =====================

-- name: GetDailyContentStats :many
SELECT day, tenant_id, content_id, views, reactions, comments, unique_viewers
FROM mv_daily_content_stats
WHERE tenant_id = $1
  AND ($2::date IS NULL OR day >= $2)
  AND ($3::date IS NULL OR day <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR content_id = $4)
ORDER BY day DESC, views DESC;

-- name: GetTopContentByViews :many
SELECT content_id, SUM(views) AS total_views, SUM(reactions) AS total_reactions, SUM(comments) AS total_comments, SUM(unique_viewers) AS total_unique_viewers
FROM mv_daily_content_stats
WHERE tenant_id = $1
  AND ($2::date IS NULL OR day >= $2)
  AND ($3::date IS NULL OR day <= $3)
GROUP BY content_id
ORDER BY total_views DESC
LIMIT $4;

-- =====================
-- Materialized View: Daily Space Stats
-- =====================

-- name: GetDailySpaceStats :many
SELECT day, tenant_id, space_id, joins, posts, active_users
FROM mv_daily_space_stats
WHERE tenant_id = $1
  AND ($2::date IS NULL OR day >= $2)
  AND ($3::date IS NULL OR day <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR space_id = $4)
ORDER BY day DESC;

-- =====================
-- Materialized View: User Engagement Stats
-- =====================

-- name: GetUserEngagementStats :many
SELECT day, tenant_id, user_id, posts_created, reactions_given, comments_made, views
FROM mv_user_engagement_stats
WHERE tenant_id = $1
  AND ($2::date IS NULL OR day >= $2)
  AND ($3::date IS NULL OR day <= $3)
  AND ($4::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR user_id = $4)
ORDER BY day DESC;

-- name: GetUserGrowthOverTime :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(*) AS new_users
FROM users
WHERE tenant_id = $1
  AND ($2::date IS NULL OR created_at::date >= $2)
  AND ($3::date IS NULL OR created_at::date <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: RefreshDailyContentStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_content_stats;

-- name: RefreshDailySpaceStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_space_stats;

-- name: RefreshUserEngagementStatsView :exec
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_user_engagement_stats;

-- =====================
-- Traffic Sources (from meta JSONB)
-- =====================

-- name: GetTrafficSources :many
SELECT meta->>'referrer' AS referrer, COUNT(*) AS visits
FROM platform_events
WHERE tenant_id = $1
  AND event_type = 'content.viewed'
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
  AND meta->>'referrer' IS NOT NULL
  AND meta->>'referrer' != ''
GROUP BY referrer
ORDER BY visits DESC
LIMIT $4;

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
-- Dashboard Aggregates
-- =====================

-- name: GetDailyActiveUsers :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(DISTINCT user_id) AS dau
FROM platform_events
WHERE tenant_id = $1
  AND user_id IS NOT NULL
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetMonthlyActiveUsers :many
SELECT date_trunc('month', created_at)::date AS month, COUNT(DISTINCT user_id) AS mau
FROM platform_events
WHERE tenant_id = $1
  AND user_id IS NOT NULL
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetContentVolumeOverTime :many
SELECT date_trunc('day', created_at)::date AS day, COUNT(*) AS content_count
FROM platform_events
WHERE tenant_id = $1
  AND event_type = 'content.created'
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY 1
ORDER BY 1 DESC;
