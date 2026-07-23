-- 000159_granular_bans.down.sql
ALTER TABLE user_moderation_actions
    DROP CONSTRAINT user_moderation_actions_action_check;
ALTER TABLE user_moderation_actions
    ADD CONSTRAINT user_moderation_actions_action_check
    CHECK (action IN ('ban', 'unban', 'warning'));

DROP INDEX IF EXISTS idx_uma_violation_type;
ALTER TABLE user_moderation_actions DROP COLUMN IF EXISTS violation_type;

DROP TABLE IF EXISTS user_operation_bans;
DROP TYPE IF EXISTS ban_operation;
