-- Down migration for 000029_platform_analytics.sql
-- Drops materialized views, security scan results, and platform events tables.
DROP MATERIALIZED VIEW IF EXISTS mv_user_engagement_stats;
DROP MATERIALIZED VIEW IF EXISTS mv_daily_space_stats;
DROP MATERIALIZED VIEW IF EXISTS mv_daily_content_stats;
DROP TABLE IF EXISTS security_scan_results;
DROP TABLE IF EXISTS platform_events;
