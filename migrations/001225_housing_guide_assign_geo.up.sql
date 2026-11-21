-- 001225_housing_guide_assign_geo.up.sql
-- Re-enable the seeded housing_guide.assign_geo ai_tools row. Migration
-- 000788 disabled it because the resolved update method's input had no
-- geo-scope field; UpdateGuideRequest now carries destination_space_id
-- (and city_space_id), so the op resolves — locked by
-- TestToolWriterOpsMatrix.
BEGIN;

UPDATE ai_tools
SET is_enabled = true, updated_at = now()
WHERE name = 'housing_guide.assign_geo';

COMMIT;
