-- Down migration for 000045_auth_password_reset.sql
-- Drops password reset tokens and email verifications tables.
DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS password_reset_tokens;
