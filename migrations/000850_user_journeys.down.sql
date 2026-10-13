-- 000850_user_journeys.down.sql
-- Revert ADR-129: drop the journeys table and the per-member cap setting.

BEGIN;

DROP TABLE IF EXISTS user_journeys;

DELETE FROM system_settings WHERE key = 'japa_stage.max_journeys';

COMMIT;
