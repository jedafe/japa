-- Down migration for 000047_password_history.sql
-- Drops the password_history table used for password reuse prevention.
DROP TABLE IF EXISTS password_history;
