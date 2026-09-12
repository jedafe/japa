-- name: ListSharePlatforms :many
-- Returns all share platforms for a tenant, ordered by position.
SELECT id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms

ORDER BY position ASC, label ASC;

-- name: ListEnabledSharePlatforms :many
-- Public query: returns only enabled share platforms, ordered by position.
SELECT id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms
WHERE enabled = true
ORDER BY position ASC, label ASC;

-- name: GetSharePlatform :one
-- Returns a single share platform by key.
SELECT id, key, label, icon, url_template, color, enabled, position, created_at, updated_at
FROM share_platforms
WHERE key = @key;

-- name: CreateSharePlatform :one
-- Creates a new share platform.
INSERT INTO share_platforms (key, label, icon, url_template, color, enabled, position) VALUES (@key, @label, @icon, @url_template, @color, @enabled, @position)

RETURNING id, key, label, icon, url_template, color, enabled, position, created_at, updated_at;

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
WHERE key = @key
RETURNING id, key, label, icon, url_template, color, enabled, position, created_at, updated_at;

-- name: DeleteSharePlatform :exec
-- Deletes a share platform by key.
DELETE FROM share_platforms
WHERE key = @key;
