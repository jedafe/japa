-- 000258_advanced_analytics.up.sql
-- Advanced Analytics Dashboard (Section 66, ADR-084).
-- Adds materialized views for top content and revenue streams,
-- and seeds analytics.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/66-advanced-analytics.md
--      japaconnect-engineering/adr/ADR-084-advanced-analytics.md

BEGIN;

-- =====================================================================
-- mv_top_content_7d — top content by engagement score over 7 days.
-- Refreshed hourly by the existing analytics:refresh_views task (§40).
-- =====================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_top_content_7d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 7
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_top_content_7d
    ON mv_top_content_7d (tenant_id, content_id);

-- =====================================================================
-- mv_top_content_30d — top content by engagement score over 30 days.
-- Refreshed hourly by the existing analytics:refresh_views task (§40).
-- =====================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_top_content_30d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 30
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_top_content_30d
    ON mv_top_content_30d (tenant_id, content_id);

-- =====================================================================
-- mv_top_content_90d — top content by engagement score over 90 days.
-- Refreshed hourly by the existing analytics:refresh_views task (§40).
-- =====================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_top_content_90d AS
SELECT content_id, tenant_id,
  SUM(views) AS total_views,
  SUM(reactions) AS total_reactions,
  SUM(comments) AS total_comments,
  SUM(views) + (SUM(reactions) * 3) + (SUM(comments) * 5) AS engagement_score
FROM mv_daily_content_stats
WHERE day >= CURRENT_DATE - 90
GROUP BY content_id, tenant_id
ORDER BY engagement_score DESC;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_top_content_90d
    ON mv_top_content_90d (tenant_id, content_id);

-- =====================================================================
-- mv_revenue_streams — monthly revenue breakdown by stream.
-- Refreshed daily by the existing analytics:refresh_views task (§40).
-- Uses the actual payments.purpose_type CHECK values (migration 000022 +
-- 000358): pro_membership, course_access, ad_slot, credit_purchase.
-- =====================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_revenue_streams AS
SELECT date_trunc('month', created_at)::date AS month, tenant_id,
  SUM(amount) FILTER (WHERE purpose_type = 'pro_membership')   AS subscription_revenue,
  SUM(amount) FILTER (WHERE purpose_type = 'course_access')    AS course_revenue,
  SUM(amount) FILTER (WHERE purpose_type = 'ad_slot')          AS ad_revenue,
  SUM(amount) FILTER (WHERE purpose_type = 'credit_purchase')  AS credit_revenue
FROM payments
WHERE status = 'completed'
GROUP BY 1, 2;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_revenue_streams
    ON mv_revenue_streams (month, tenant_id);

-- =====================================================================
-- system_settings — 1 analytics.* key (Section 66, ADR-084)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'analytics.export_enabled', '"true"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 analytics.* nodes (Section 66, ADR-084)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('analytics.advanced', 'View advanced analytics dashboard (admin only)'),
    ('analytics.export',   'Download CSV exports')
ON CONFLICT (key) DO NOTHING;

-- Grant both analytics.* to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('analytics.advanced', 'analytics.export')
ON CONFLICT DO NOTHING;

COMMIT;
