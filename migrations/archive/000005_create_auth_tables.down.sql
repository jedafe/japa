-- Down migration for 000005_create_auth_tables.sql
-- Drops auth tables: oauth_accounts, totp_credentials, refresh_tokens.
-- All reference users/tenants (parents), so order among them doesn't matter.
DROP TABLE IF EXISTS oauth_accounts;
DROP TABLE IF EXISTS totp_credentials;
DROP TABLE IF EXISTS refresh_tokens;
