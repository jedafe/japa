-- 000623_seed_japa_stage_recommend_notification_type.up.sql
-- §87 Japa Stage Progress Profiles (ADR-095) — seed the notification type
-- used by the japa_stage:recommend Asynq task. Ref: sections/87-japa-stage-profiles.md

BEGIN;

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push)
VALUES
    ('japa_stage.recommend', 'Japa Stage Recommendations', 'Recommended tools and next steps for your current japa stage', true, false, false),
    ('japa_stage.readiness_prompt', 'Readiness Assessment Prompt', 'Prompt to take the Japa Readiness Assessment when planning', true, false, false),
    ('japa_stage.achievement_prompt', 'Achievement Feed Prompt', 'Prompt to share a relocation milestone in the Achievement Feed', true, false, false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
