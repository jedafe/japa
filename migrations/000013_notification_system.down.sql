-- Down migration for 000013_notification_system.sql
-- Drops notification batch windows, preferences, notifications, and notification types tables.
DROP TABLE IF EXISTS notification_batch_windows;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS notification_types;
