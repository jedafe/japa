-- 000553_jce_scores.down.sql

BEGIN;

DROP TABLE IF EXISTS comparison_scores CASCADE;

UPDATE comparison_content_metadata
SET config = config - 'profile_id' - 'weights_snapshot' - 'recommendation_summary' - 'last_scored_at';

COMMIT;
