-- devices.sql — Mobile Push / Device Tokens queries (Section 77, ADR-074).
-- Covers device token registration, deregistration, listing, and
-- push notification dispatch helpers.
-- Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md

-- =====================================================================
-- Register or update a device token (upsert).
-- =====================================================================

-- name: UpsertDeviceToken :one
INSERT INTO device_tokens (tenant_id, user_id, device_id, token, platform, app_version, is_active)
VALUES (@tenant_id, @user_id, @device_id, @token, @platform, sqlc.narg(app_version), true)
ON CONFLICT (tenant_id, user_id, device_id)
DO UPDATE SET
    token        = EXCLUDED.token,
    platform     = EXCLUDED.platform,
    app_version  = EXCLUDED.app_version,
    is_active    = true,
    last_used_at = now(),
    updated_at   = now()
RETURNING *;

-- =====================================================================
-- Deregister a device token (set is_active = false).
-- =====================================================================

-- name: DeregisterDeviceToken :exec
UPDATE device_tokens
SET is_active = false, updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND device_id = @device_id;

-- =====================================================================
-- List a user's active device tokens.
-- =====================================================================

-- name: ListUserDeviceTokens :many
SELECT * FROM device_tokens
WHERE tenant_id = @tenant_id AND user_id = @user_id
  AND (@active_only::boolean = false OR is_active = true)
ORDER BY created_at DESC;

-- =====================================================================
-- Get all active device tokens for a user (for push dispatch).
-- =====================================================================

-- name: GetActiveDeviceTokensForUser :many
SELECT * FROM device_tokens
WHERE tenant_id = @tenant_id AND user_id = @user_id AND is_active = true
ORDER BY last_used_at DESC;

-- =====================================================================
-- Mark a device token as inactive (invalid token feedback from FCM/APNs).
-- =====================================================================

-- name: MarkDeviceTokenInactive :exec
UPDATE device_tokens
SET is_active = false, updated_at = now()
WHERE token = @token;

-- =====================================================================
-- Update last_used_at on a device token (heartbeat).
-- =====================================================================

-- name: TouchDeviceToken :exec
UPDATE device_tokens
SET last_used_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND device_id = @device_id AND is_active = true;

-- =====================================================================
-- Get a device token by ID (for admin test push).
-- =====================================================================

-- name: GetDeviceTokenByDeviceID :one
SELECT * FROM device_tokens
WHERE tenant_id = @tenant_id AND device_id = @device_id AND is_active = true;
