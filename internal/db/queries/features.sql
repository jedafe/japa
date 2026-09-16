-- features.sql — Feature Flags queries (global scope).

-- name: GetFeatureFlag :one
SELECT key, enabled, config, updated_at, updated_by
FROM feature_flags
WHERE key = $1;

-- name: ListFeatureFlags :many
SELECT key, enabled, config, updated_at, updated_by
FROM feature_flags
ORDER BY key;

-- name: UpsertFeatureFlagEnabled :exec
INSERT INTO feature_flags (key, enabled, updated_by) VALUES ($1, $2, $3)

ON CONFLICT (key)
DO UPDATE SET enabled = $2, updated_by = $3, updated_at = now();

-- name: UpsertFeatureFlagWithConfig :exec
INSERT INTO feature_flags (key, enabled, config, updated_by) VALUES ($1, $2, $3, $4)

ON CONFLICT (key)
DO UPDATE SET enabled = $2, config = $3, updated_by = $4, updated_at = now();

-- name: DeleteFeatureFlag :exec
DELETE FROM feature_flags
WHERE key = $1;
