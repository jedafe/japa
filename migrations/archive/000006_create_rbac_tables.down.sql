-- Down migration for 000006_create_rbac_tables.sql
-- Drops RBAC join tables first, then parent tables.
-- Seeded system default groups are removed when user_groups is dropped.
DROP TABLE IF EXISTS user_group_memberships;
DROP TABLE IF EXISTS group_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS user_groups;
