-- 000696_seed_calendar_categories_setting.down.sql
-- Renumbered from 000458 (duplicate version prefix). See the .up file.
-- 000458_seed_calendar_categories_setting.down.sql
-- Reverses 000458_seed_calendar_categories_setting.up.sql.

DELETE FROM system_settings WHERE key = 'calendar.categories';
