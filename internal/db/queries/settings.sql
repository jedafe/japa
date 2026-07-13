-- System settings queries (migrated from inline SQL in config/settings.go and control/handlers.go)

-- name: GetSettingValue :one
SELECT value FROM system_settings WHERE tenant_id = $1 AND key = $2;

-- name: GetPublicSettings :many
SELECT key, value FROM system_settings WHERE tenant_id = $1 AND is_public = true ORDER BY key;

-- name: ListSystemSettings :many
SELECT key, value, is_public, updated_at FROM system_settings WHERE tenant_id = $1 ORDER BY key;

-- name: UpsertSystemSetting :exec
INSERT INTO system_settings (tenant_id, key, value, is_public, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, is_public = $4, updated_at = now();

-- name: DeleteSystemSetting :exec
DELETE FROM system_settings WHERE tenant_id = $1 AND key = $2;
