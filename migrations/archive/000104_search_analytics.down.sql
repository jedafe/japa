-- Down migration for 000104: search_analytics.
-- Drops the search_analytics table (Section 53).
DROP INDEX IF EXISTS idx_search_analytics_zero;
DROP INDEX IF EXISTS idx_search_analytics_query;
DROP TABLE IF EXISTS search_analytics;
