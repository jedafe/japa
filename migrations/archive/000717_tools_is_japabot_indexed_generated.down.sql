-- 000717_tools_is_japabot_indexed_generated.down.sql
-- Restore plain boolean columns (values recompute to false; historical
-- indexed state is not preserved — same trade-off as 000375.down).

BEGIN;

ALTER TABLE crs_rulesets DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE crs_rulesets ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE crs_draws DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE crs_draws ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE pof_requirements DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE pof_requirements ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE eca_guides DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE eca_guides ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE sop_samples DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE sop_samples ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

COMMIT;
