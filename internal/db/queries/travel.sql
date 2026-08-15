-- travel.sql — Travel Search & Compare queries (Section 67, ADR-085).
-- Covers travel provider CRUD and admin oversight.
-- Ref: japaconnect-engineering/sections/67-travel-search.md
--      japaconnect-engineering/adr/ADR-085-travel-search.md

-- =====================================================================
-- Create a travel provider.
-- =====================================================================

-- name: CreateTravelProvider :one
INSERT INTO travel_providers (tenant_id, name, slug, provider_type, api_config, affiliate_config, is_active)
VALUES (@tenant_id, @name, @slug, @provider_type, sqlc.narg(api_config), sqlc.narg(affiliate_config), @is_active)
RETURNING *;

-- =====================================================================
-- Update a travel provider.
-- =====================================================================

-- name: UpdateTravelProvider :one
UPDATE travel_providers
SET
    name             = COALESCE(sqlc.narg(name), name),
    provider_type    = COALESCE(sqlc.narg(provider_type), provider_type),
    api_config       = COALESCE(sqlc.narg(api_config), api_config),
    affiliate_config = COALESCE(sqlc.narg(affiliate_config), affiliate_config),
    is_active        = COALESCE(sqlc.narg(is_active), is_active)
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- =====================================================================
-- Get a single travel provider by ID.
-- =====================================================================

-- name: GetTravelProviderByID :one
SELECT * FROM travel_providers
WHERE tenant_id = @tenant_id AND id = @id;

-- =====================================================================
-- List travel providers with optional search and active-status filter.
-- q: ILIKE search on name or slug (empty = no filter).
-- is_active_filter: tri-state — 'true' = only active, 'false' = only
--   inactive, '' (empty) = no filter (all statuses).
-- provider_type: filter by exact type (empty = all).
-- =====================================================================

-- name: ListTravelProviders :many
SELECT * FROM travel_providers
WHERE tenant_id = @tenant_id
  AND (@is_active_filter::text = '' OR is_active = (@is_active_filter::text = 'true'))
  AND (@provider_type::text = '' OR provider_type = @provider_type)
  AND (@q::text = '' OR name ILIKE '%' || @q::text || '%' OR slug ILIKE '%' || @q::text || '%')
ORDER BY name ASC;

-- =====================================================================
-- Get the active provider for a given type (flight, hotel, or both).
-- Used by the search proxy to find the configured provider.
-- =====================================================================

-- name: GetActiveTravelProvider :one
SELECT * FROM travel_providers
WHERE tenant_id = @tenant_id
  AND is_active = true
  AND (provider_type = @provider_type OR provider_type = 'both')
ORDER BY
    CASE WHEN provider_type = @provider_type THEN 0 ELSE 1 END,
    created_at DESC
LIMIT 1;
