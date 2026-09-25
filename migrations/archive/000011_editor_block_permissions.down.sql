-- Down migration for 000011_editor_block_permissions.sql
-- Drops the editor_block_permissions table (seeded rows removed with it).
DROP TABLE IF EXISTS editor_block_permissions;
