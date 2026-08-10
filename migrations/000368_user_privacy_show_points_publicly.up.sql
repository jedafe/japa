-- 000368_user_privacy_show_points_publicly.up.sql
-- Add show_points_publicly to user_privacy_settings for consent-based
-- public Points display on user profiles (Section 28).
-- Default false — users must explicitly opt in via PATCH /me/privacy.
-- Credits are never exposed publicly regardless of this setting.

ALTER TABLE user_privacy_settings
    ADD COLUMN IF NOT EXISTS show_points_publicly boolean NOT NULL DEFAULT false;
