-- name: ListSharePlatforms :many
-- Returns all share platforms for a tenant, ordered by position.
SELECT id, tenant_id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms
WHERE tenant_id = @tenant_id
ORDER BY position ASC, label ASC;

-- name: ListEnabledSharePlatforms :many
-- Public query: returns only enabled share platforms, ordered by position.
SELECT id, tenant_id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms
WHERE tenant_id = @tenant_id
  AND enabled = true
ORDER BY position ASC, label ASC;

-- name: GetSharePlatform :one
-- Returns a single share platform by key.
SELECT id, tenant_id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms
WHERE tenant_id = @tenant_id
  AND key = @key;

-- name: CreateSharePlatform :one
-- Creates a new share platform.
INSERT INTO share_platforms (tenant_id, key, label, icon, url_template, color, enabled, position)
VALUES (@tenant_id, @key, @label, @icon, @url_template, @color, @enabled, @position)
RETURNING id, tenant_id, key, label, icon, url_template, color, enabled, position, created_at, updated_at;

-- name: UpdateSharePlatform :one
-- Updates a share platform by key.
UPDATE share_platforms
SET label = @label,
    icon = @icon,
    url_template = @url_template,
    color = @color,
    enabled = @enabled,
    position = @position,
    updated_at = now()
WHERE tenant_id = @tenant_id
  AND key = @key
RETURNING id, tenant_id, key, label, icon, url_template, color, enabled, position, created_at, updated_at;

-- name: DeleteSharePlatform :exec
-- Deletes a share platform by key.
DELETE FROM share_platforms
WHERE tenant_id = @tenant_id
  AND key = @key;
