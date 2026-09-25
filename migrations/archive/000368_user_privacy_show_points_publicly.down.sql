-- 000368_user_privacy_show_points_publicly.down.sql
ALTER TABLE user_privacy_settings
    DROP COLUMN IF EXISTS show_points_publicly;
