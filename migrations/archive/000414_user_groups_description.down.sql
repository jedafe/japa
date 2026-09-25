-- Revert: remove description column from user_groups.
ALTER TABLE user_groups DROP COLUMN IF EXISTS description;
