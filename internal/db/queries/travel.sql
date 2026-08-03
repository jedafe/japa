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
-- List travel providers (optionally active only).
-- =====================================================================

-- name: ListTravelProviders :many
SELECT * FROM travel_providers
WHERE tenant_id = @tenant_id
  AND (@active_only::boolean = false OR is_active = true)
  AND (@provider_type::text = '' OR provider_type = @provider_type)
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
