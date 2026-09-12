-- System settings queries (global scope after multi-tenancy removal).

-- name: GetSettingValue :one
SELECT value FROM system_settings WHERE key = $1;

-- name: GetPublicSettings :many
SELECT key, value FROM system_settings WHERE is_public = true ORDER BY key;

-- name: ListSystemSettings :many
SELECT key, value, is_public, updated_at FROM system_settings ORDER BY key;

-- name: UpsertSystemSetting :exec
INSERT INTO system_settings (key, value, is_public, updated_at) VALUES ($1, $2, $3, now())

ON CONFLICT (key) DO UPDATE SET value = $2, is_public = $3, updated_at = now();

-- name: UpsertSystemSettingPreservePublic :exec
INSERT INTO system_settings (key, value, is_public, created_at, updated_at) VALUES ($1, $2, false, now(), now())

ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteSystemSetting :exec
DELETE FROM system_settings WHERE key = $1;
