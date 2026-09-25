-- 000437_add_user_blocks_composite_index.up.sql
-- Adds a composite index for user_blocks to serve the user-facing
-- ListBlockedUsers query (WHERE blocker_id = $1 AND tenant_id = $2
-- ORDER BY created_at DESC) without a sequential scan.
--
-- The standalone idx_user_blocks_tenant was dropped in migration 000433
-- because it had zero selectivity at single-tenant scale. This composite
-- index replaces it with something better: it serves the actual user-facing
-- query pattern (blocker_id + tenant_id + created_at ordering) which the
-- standalone index never could.
--
-- The admin-only ListAllUserBlocks (WHERE tenant_id = $1 only) remains a
-- seq scan — acceptable because it is admin-only, paginated, and low-traffic.
-- If it ever becomes slow, add (tenant_id, created_at DESC) then.

CREATE INDEX IF NOT EXISTS idx_user_blocks_blocker_tenant
    ON user_blocks (blocker_id, tenant_id, created_at DESC);
