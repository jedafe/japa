-- Reversal: re-enable visa-types creation. The test fixture categories are
-- not restored (they were garbage data, not real content).
BEGIN;

DELETE FROM comparison_type_access ta
USING comparison_type_definitions td
WHERE ta.type_id = td.id AND td.slug = 'visa-types';

COMMIT;
