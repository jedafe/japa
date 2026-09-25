-- 000511_community_locator_gist_point_index.down.sql
-- Reverse: drop the GiST point index. The btree index from 000343
-- remains, restoring the bounding-box approximation behavior.

BEGIN;

DROP INDEX IF EXISTS idx_community_orgs_gist_point;

COMMIT;
