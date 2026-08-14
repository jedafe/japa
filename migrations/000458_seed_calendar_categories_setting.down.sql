-- 000458_seed_calendar_categories_setting.down.sql
-- Reverses 000458_seed_calendar_categories_setting.up.sql.

DELETE FROM system_settings WHERE key = 'calendar.categories';
