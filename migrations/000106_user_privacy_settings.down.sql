-- Down migration for 000106_user_privacy_settings.sql
-- Drops the user_privacy_settings table for /me/privacy endpoints.
DROP TABLE IF EXISTS user_privacy_settings;
