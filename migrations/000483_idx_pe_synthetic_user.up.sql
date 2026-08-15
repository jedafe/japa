-- 000483_idx_pe_synthetic_user.up.sql
-- Add a composite index on platform_events (tenant_id, user_id, created_at DESC)
-- filtered to is_synthetic = true. This directly supports the LATERAL join in
-- ListAIPersonas (internal/db/queries/personas.sql) which counts synthetic
-- events and finds the last action timestamp per persona backing user:
--
--   SELECT COUNT(*), MAX(pe.created_at)
--   FROM platform_events pe
--   WHERE pe.tenant_id = ap.tenant_id
--     AND pe.user_id = ap.user_id
--     AND pe.is_synthetic = true
--
-- The existing idx_pe_synthetic (tenant_id, event_type, created_at DESC) has
-- the wrong leading-column order for this query — it filters by event_type,
-- not user_id, so the LATERAL subquery scans all synthetic event types for
-- the tenant instead of index-seeking the specific user. With this new index
-- the subquery does an index range scan on (tenant_id, user_id) and reads
-- created_at from the index for MAX(), enabling an index-only scan for the
-- count (BUG-PER-009).
--
-- Ref: japaconnect-engineering/sections/68-ai-personas.md
--      japaconnect-engineering/sections/40-platform-analytics-reports.md

BEGIN;

CREATE INDEX IF NOT EXISTS idx_pe_synthetic_user
    ON platform_events (tenant_id, user_id, created_at DESC)
    WHERE is_synthetic = true;

COMMIT;
