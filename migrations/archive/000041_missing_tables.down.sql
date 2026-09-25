-- Down migration for 000041_missing_tables.sql
-- Drops missing tables added in the roadmap audit and removes the
-- max_spaces_creatable column from user_groups.
DROP TABLE IF EXISTS onboarding_templates;
DROP TABLE IF EXISTS custom_field_templates;
DROP TABLE IF EXISTS user_group_promotions;
DROP TABLE IF EXISTS page_builder_pages;
DROP TABLE IF EXISTS editor_block_permissions;
DROP TABLE IF EXISTS content_spaces;
DROP TABLE IF EXISTS space_moderators;
ALTER TABLE user_groups DROP COLUMN IF EXISTS max_spaces_creatable;
