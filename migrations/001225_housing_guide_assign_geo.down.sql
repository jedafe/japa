-- 001225 down: restore the 000788 disabled state.
BEGIN;

UPDATE ai_tools
SET is_enabled = false, updated_at = now()
WHERE name = 'housing_guide.assign_geo';

COMMIT;
