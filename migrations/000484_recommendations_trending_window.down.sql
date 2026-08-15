-- 000484_recommendations_trending_window.down.sql
-- Reverses 000484_recommendations_trending_window.up.sql.

DELETE FROM system_settings
WHERE key = 'geo_personalization.recommendations_trending_window'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';
