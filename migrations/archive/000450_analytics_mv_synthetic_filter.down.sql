-- 000450_analytics_mv_synthetic_filter.down.sql
-- Revert the materialized views to their original (pre-P10-filter) definitions
-- from migration 000029. This re-introduces synthetic events into real-metric
-- dashboards, which violates P10/ADR-068 §4 — only run this to roll back.

BEGIN;

-- =====================================================================
-- Drop dependent MVs first — mv_top_content_* depend on mv_daily_content_stats
-- and must be dropped before it can be dropped, then recreated after.
-- =====================================================================
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_90d;
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_30d;
DROP MATERIALIZED VIEW IF EXISTS mv_top_content_7d;

-- =====================================================================
-- mv_daily_content_stats — revert to original (no is_synthetic filter)
-- =====================================================================
DROP MATERIALIZED VIEW IF EXISTS mv_daily_content_stats;

CREATE MATERIALIZED VIEW mv_daily_content_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    entity_id AS content_id,
    COUNT(*) FILTER (WHERE event_type = 'content.viewed') AS views,
    COUNT(*) FILTER (WHERE event_type = 'reaction.given') AS reactions,
    COUNT(*) FILTER (WHERE event_type = 'comment.created') AS comments,
    COUNT(DISTINCT user_id) AS unique_viewers
FROM platform_events
WHERE event_type IN ('content.viewed', 'reaction.given', 'comment.created')
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_daily_content_stats (day, tenant_id, content_id);

-- =====================================================================
-- mv_daily_space_stats — revert to original (no is_synthetic filter)
-- =====================================================================
DROP MATERIALIZED VIEW IF EXISTS mv_daily_space_stats;

CREATE MATERIALIZED VIEW mv_daily_space_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    entity_id AS space_id,
    COUNT(*) FILTER (WHERE event_type = 'space.joined') AS joins,
    COUNT(*) FILTER (WHERE event_type = 'content.created') AS posts,
    COUNT(DISTINCT user_id) AS active_users
FROM platform_events
WHERE event_type IN ('space.joined', 'content.created')
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_daily_space_stats (day, tenant_id, space_id);

-- =====================================================================
-- mv_user_engagement_stats — revert to original (no is_synthetic filter)
-- =====================================================================
DROP MATERIALIZED VIEW IF EXISTS mv_user_engagement_stats;

CREATE MATERIALIZED VIEW mv_user_engagement_stats AS
SELECT
    date_trunc('day', created_at)::date AS day,
    tenant_id,
    user_id,
    COUNT(*) FILTER (WHERE event_type = 'content.created') AS posts_created,
    COUNT(*) FILTER (WHERE event_type = 'reaction.given') AS reactions_given,
    COUNT(*) FILTER (WHERE event_type = 'comment.created') AS comments_made,
    COUNT(*) FILTER (WHERE event_type = 'content.viewed') AS views
FROM platform_events
WHERE user_id IS NOT NULL
GROUP BY 1, 2, 3;

CREATE UNIQUE INDEX ON mv_user_engagement_stats (day, tenant_id, user_id);

-- =====================================================================
-- Recreate dependent MVs — they query mv_daily_content_stats which is now
-- reverted to the original (no is_synthetic filter) definition.
-- =====================================================================
CREATE MATERIALIZED VIEW mv_top_content_7d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 7
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX idx_mv_top_content_7d
    ON mv_top_content_7d (tenant_id, content_id);

CREATE MATERIALIZED VIEW mv_top_content_30d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 30
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX idx_mv_top_content_30d
    ON mv_top_content_30d (tenant_id, content_id);

CREATE MATERIALIZED VIEW mv_top_content_90d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 90
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX idx_mv_top_content_90d
    ON mv_top_content_90d (tenant_id, content_id);

COMMIT;
