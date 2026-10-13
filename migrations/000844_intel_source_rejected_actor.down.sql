-- 000844_intel_source_rejected_actor.down.sql
--
-- Drop the rejected decision-actor columns added by the up migration.

ALTER TABLE public.intel_sources
    DROP CONSTRAINT IF EXISTS intel_sources_rejected_by_fkey,
    DROP COLUMN IF EXISTS rejected_by,
    DROP COLUMN IF EXISTS rejected_at;
