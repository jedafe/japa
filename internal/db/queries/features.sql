-- features.sql — Feature Flags queries (Section 42, ADR-042).
-- Covers single-flag lookup, batch lookup, upsert (toggle + full config),
-- delete, and list-all for the admin panel.
-- Ref: japaconnect-engineering/sections/42-feature-flags.md

-- =====================================================================
-- Get a single feature flag by key + tenant.
-- =====================================================================

-- name: GetFeatureFlag :one
SELECT key, tenant_id, enabled, config, updated_at, updated_by
FROM feature_flags
WHERE key = $1 AND tenant_id = $2;

-- =====================================================================
-- Get all feature flags for a tenant in one round-trip (M2: eliminates
-- the N+1 query pattern in GetAllEnabled).
-- =====================================================================

-- name: ListFeatureFlagsByTenant :many
SELECT key, tenant_id, enabled, config, updated_at, updated_by
FROM feature_flags
WHERE tenant_id = $1;

-- =====================================================================
-- Upsert a feature flag's enabled state (toggle, no config change).
-- Used by SetEnabled (PATCH /admin/feature-flags/{flag_name}).
-- =====================================================================

-- name: UpsertFeatureFlagEnabled :exec
INSERT INTO feature_flags (key, tenant_id, enabled, updated_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key, tenant_id)
DO UPDATE SET enabled = $3, updated_by = $4, updated_at = now();

-- =====================================================================
-- Upsert a feature flag with full config metadata (create + update).
-- Used by UpdateFlag (POST/PUT /admin/feature-flags).
-- =====================================================================

-- name: UpsertFeatureFlagWithConfig :exec
INSERT INTO feature_flags (key, tenant_id, enabled, config, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (key, tenant_id)
DO UPDATE SET enabled = $3, config = $4, updated_by = $5, updated_at = now();

-- =====================================================================
-- Delete a tenant's feature flag override (reverts to registry default).
-- Used by DeleteFlag (DELETE /admin/feature-flags/{flag_name}).
-- =====================================================================

-- name: DeleteFeatureFlag :exec
DELETE FROM feature_flags
WHERE key = $1 AND tenant_id = $2;
