-- Down migration for 000380_seed_calendar_notification_type
-- Removes the calendar_reminder notification type.
-- Existing notifications with this type are deleted first to satisfy
-- the FK constraint from notifications.notification_type.

DELETE FROM notifications WHERE notification_type = 'calendar_reminder';
DELETE FROM notification_types WHERE key = 'calendar_reminder';
