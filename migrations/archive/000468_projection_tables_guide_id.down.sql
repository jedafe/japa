-- 000468_projection_tables_guide_id.down.sql
-- Reverses: removes guide_id from visa_routes/housing_guides/credential_pathways
-- and restores scholarships.guide_id to NOT NULL. Note: restoring NOT NULL
-- will fail if any NULL guide_id rows exist — those must be backfilled first.

BEGIN;

ALTER TABLE credential_pathways DROP COLUMN IF EXISTS guide_id;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS guide_id;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS guide_id;

-- Restore scholarships.guide_id to NOT NULL (fails if NULLs exist).
ALTER TABLE scholarships ALTER COLUMN guide_id SET NOT NULL;
DROP INDEX IF EXISTS idx_scholarships_guide;
CREATE UNIQUE INDEX idx_scholarships_guide ON scholarships (guide_id);

COMMIT;
