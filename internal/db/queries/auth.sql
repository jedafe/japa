-- Auth queries: refresh tokens, TOTP, OAuth, password reset, email verification, API keys, user management.

-- =====================
-- User Management
-- =====================

-- name: CreateUser :one
-- New users start on the regular tier (BUG-019).
INSERT INTO users (tenant_id, email, username, password_hash, profile_type)
VALUES (@tenant_id, @email, @username, @password_hash, 'regular')
RETURNING id;

-- name: GetUserByEmail :one
SELECT id, tenant_id, email, password_hash, username, profile_type, status, email_verified_at, is_bot
FROM users
WHERE tenant_id = @tenant_id AND email = @email
LIMIT 1;

-- name: UpdateUserPasswordByUserID :exec
-- Sets password_changed_at so max-password-age policies can be enforced
-- (Ref: sections/02-authentication-sessions.md).
UPDATE users
SET password_hash = @password_hash, password_changed_at = now(), updated_at = now()
WHERE id = @user_id;

-- name: MarkEmailVerifiedByUserID :exec
UPDATE users
SET email_verified_at = now(), updated_at = now()
WHERE id = @user_id AND email_verified_at IS NULL;

-- =====================
-- Refresh Tokens
-- =====================

-- name: CreateRefreshToken :exec
-- device_name/ip_address captured at issue time per JEOS §50.
INSERT INTO refresh_tokens (tenant_id, user_id, token_hash, expires_at, device_name, ip_address)
VALUES (@tenant_id, @user_id, @token_hash, @expires_at, @device_name, @ip_address);

-- name: TouchRefreshToken :exec
-- Stamp last_used_at when a refresh token is exchanged for a new access
-- token (POST /auth/refresh) so /me/sessions can show last activity.
UPDATE refresh_tokens
SET last_used_at = now(), updated_at = now()
WHERE token_hash = @token_hash AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: GetRefreshTokenByHash :one
SELECT id, tenant_id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at
FROM refresh_tokens
WHERE token_hash = @token_hash AND tenant_id = @tenant_id;

-- name: GetRefreshTokenByHashGlobal :one
SELECT id, tenant_id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at
FROM refresh_tokens
WHERE token_hash = @token_hash
LIMIT 1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE token_hash = @token_hash AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: RevokeRefreshTokenByHash :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE token_hash = @token_hash AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < now() OR revoked_at IS NOT NULL;

-- =====================
-- TOTP Credentials
-- =====================

-- name: GetTOTPCredentials :one
SELECT id, tenant_id, user_id, secret_encrypted, enabled_at, backup_codes, created_at, updated_at
FROM totp_credentials
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: CreateTOTPCredentials :one
INSERT INTO totp_credentials (tenant_id, user_id, secret_encrypted, backup_codes)
VALUES (@tenant_id, @user_id, @secret_encrypted, @backup_codes)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    secret_encrypted = EXCLUDED.secret_encrypted,
    backup_codes = EXCLUDED.backup_codes,
    updated_at = now()
RETURNING id, tenant_id, user_id, secret_encrypted, enabled_at, backup_codes, created_at, updated_at;

-- name: EnableTOTP :exec
UPDATE totp_credentials
SET enabled_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND enabled_at IS NULL;

-- name: DeleteTOTPCredentials :exec
DELETE FROM totp_credentials
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: CheckTOTPEnabled :one
SELECT EXISTS(
    SELECT 1 FROM totp_credentials
    WHERE tenant_id = @tenant_id AND user_id = @user_id AND enabled_at IS NOT NULL
) AS enabled;

-- =====================
-- OAuth Accounts
-- =====================

-- name: GetOAuthAccountByProvider :one
SELECT id, tenant_id, user_id, provider, provider_user_id, created_at
FROM oauth_accounts
WHERE tenant_id = @tenant_id AND provider = @provider AND provider_user_id = @provider_user_id;

-- name: GetOAuthAccountsByUser :many
SELECT id, tenant_id, user_id, provider, provider_user_id, created_at
FROM oauth_accounts
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: LinkOAuthAccount :one
INSERT INTO oauth_accounts (tenant_id, user_id, provider, provider_user_id)
VALUES (@tenant_id, @user_id, @provider, @provider_user_id)
ON CONFLICT (tenant_id, provider, provider_user_id) DO UPDATE SET
    user_id = EXCLUDED.user_id
RETURNING id, tenant_id, user_id, provider, provider_user_id, created_at;

-- name: UnlinkOAuthAccount :exec
DELETE FROM oauth_accounts
WHERE tenant_id = @tenant_id AND user_id = @user_id AND provider = @provider;

-- =====================
-- Password Reset Tokens
-- =====================

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (tenant_id, user_id, token_hash, expires_at)
VALUES (@tenant_id, @user_id, @token_hash, @expires_at);

-- name: GetPasswordResetTokenByHash :one
SELECT id, tenant_id, user_id, token_hash, expires_at, used_at, created_at
FROM password_reset_tokens
WHERE token_hash = @token_hash AND tenant_id = @tenant_id;

-- name: GetPasswordResetTokenByHashGlobal :one
SELECT tenant_id, user_id, expires_at, used_at
FROM password_reset_tokens
WHERE token_hash = @token_hash
LIMIT 1;

-- name: UsePasswordResetToken :exec
UPDATE password_reset_tokens
SET used_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND used_at IS NULL;

-- name: UsePasswordResetTokenByHash :exec
UPDATE password_reset_tokens
SET used_at = now()
WHERE token_hash = @token_hash AND used_at IS NULL;

-- name: InvalidateUserPasswordResetTokens :exec
UPDATE password_reset_tokens
SET used_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND used_at IS NULL;

-- =====================
-- Email Verifications
-- =====================

-- name: CreateEmailVerification :exec
INSERT INTO email_verifications (tenant_id, user_id, email, token_hash, expires_at)
VALUES (@tenant_id, @user_id, @email, @token_hash, @expires_at);

-- name: GetEmailVerificationByHash :one
SELECT id, tenant_id, user_id, email, token_hash, expires_at, verified_at, created_at
FROM email_verifications
WHERE token_hash = @token_hash AND tenant_id = @tenant_id;

-- name: GetEmailVerificationByHashGlobal :one
SELECT tenant_id, user_id, expires_at, verified_at
FROM email_verifications
WHERE token_hash = @token_hash
LIMIT 1;

-- name: VerifyEmail :exec
UPDATE email_verifications
SET verified_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND verified_at IS NULL;

-- name: InvalidateUserEmailVerifications :exec
UPDATE email_verifications
SET verified_at = now()
WHERE user_id = @user_id AND tenant_id = @tenant_id AND verified_at IS NULL;

-- =====================
-- API Keys
-- =====================

-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, expires_at)
VALUES (@tenant_id, @user_id, @name, @key_prefix, @key_hash, @scopes, @rate_limit_tier, sqlc.narg(expires_at))
RETURNING id, tenant_id, user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at;

-- name: GetAPIKeyByHash :one
SELECT id, tenant_id, user_id, name, key_prefix, key_hash, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
FROM api_keys
WHERE key_hash = @key_hash AND tenant_id = @tenant_id AND status = 'active';

-- name: ListAPIKeysByUser :many
SELECT id, tenant_id, user_id, name, key_prefix, scopes, rate_limit_tier, status, last_used_at, request_count, expires_at, created_at, updated_at
FROM api_keys
WHERE user_id = @user_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys
SET status = 'revoked', updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;

-- name: IncrementAPIKeyUsage :exec
UPDATE api_keys
SET last_used_at = now(), request_count = request_count + 1, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: DeleteExpiredAPIKeys :exec
DELETE FROM api_keys
WHERE expires_at IS NOT NULL AND expires_at < now() AND status = 'active';

-- =====================
-- Password History
-- =====================

-- name: InsertPasswordHistory :exec
INSERT INTO password_history (tenant_id, user_id, password_hash)
VALUES (@tenant_id, @user_id, @password_hash);

-- name: GetRecentPasswordHashes :many
SELECT password_hash
FROM password_history
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @hash_limit;
