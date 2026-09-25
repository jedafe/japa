-- Down migration for 000003_create_users_table.sql
-- Drops user_profiles then users (child before parent).
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS users;
