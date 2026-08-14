-- Down migration for 000102_user_space_mutes.sql
-- Drops space_mutes and user_mutes tables for the mute system.
DROP TABLE IF EXISTS space_mutes;
DROP TABLE IF EXISTS user_mutes;
