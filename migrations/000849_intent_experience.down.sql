-- 000849_intent_experience.down.sql
-- Revert ADR-128 seed data: remove the intent_matrix setting and strip the
-- intent-scoped keys added to stage_tool_matrix.

BEGIN;

DELETE FROM system_settings WHERE key = 'geo_personalization.intent_matrix';

UPDATE system_settings
SET value = value - 'visit:*' - 'business:*' - 'study:*' - 'work:*',
    updated_at = now()
WHERE key = 'geo_personalization.stage_tool_matrix';

COMMIT;
