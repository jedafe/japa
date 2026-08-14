-- Down migration for 000055_pagebuilder.sql
-- Drops pages, themes, templates, and blocks tables for the page builder.
DROP TABLE IF EXISTS pages;
DROP TABLE IF EXISTS pagebuilder_themes;
DROP TABLE IF EXISTS pagebuilder_templates;
DROP TABLE IF EXISTS pagebuilder_blocks;
