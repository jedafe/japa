-- 000468_projection_tables_guide_id.up.sql
-- Approach B: Align all projection tables with a nullable guide_id FK to
-- contents(id). Intelligence promoters write projection rows with
-- guide_id = NULL; the AI Content Studio later generates the editorial
-- contents row and updates guide_id to link them.
-- Ref: ADR-076 Amendment 1, ADR-088.

BEGIN;

-- 1. scholarships: make guide_id nullable (was NOT NULL from migration 000429).
--    The unique index idx_scholarships_guide must become partial since NULLs
--    are now allowed and multiple NULLs would violate a non-partial UNIQUE.
ALTER TABLE scholarships ALTER COLUMN guide_id DROP NOT NULL;
DROP INDEX IF EXISTS idx_scholarships_guide;
CREATE UNIQUE INDEX idx_scholarships_guide
    ON scholarships (guide_id) WHERE guide_id IS NOT NULL;

-- 2. visa_routes: add guide_id (nullable FK to contents).
ALTER TABLE visa_routes
    ADD COLUMN guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;

-- 3. housing_guides: add guide_id (nullable FK to contents).
ALTER TABLE housing_guides
    ADD COLUMN guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;

-- 4. credential_pathways: add guide_id (nullable FK to contents).
ALTER TABLE credential_pathways
    ADD COLUMN guide_id uuid REFERENCES contents(id) ON DELETE SET NULL;

COMMIT;
