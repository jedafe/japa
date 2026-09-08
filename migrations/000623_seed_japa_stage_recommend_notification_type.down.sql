-- 000623_seed_japa_stage_recommend_notification_type.down.sql

BEGIN;

DELETE FROM notification_types WHERE key IN ('japa_stage.recommend', 'japa_stage.readiness_prompt', 'japa_stage.achievement_prompt');

COMMIT;
