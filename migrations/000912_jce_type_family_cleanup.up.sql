-- 000912_jce_type_family_cleanup.up.sql
-- Type-selector cleanup (user request):
--   1. city-prices + city-transportation-prices duplicate the wiki-bound
--      city-cost-of-living / col-transportation types — hide them from the
--      picker via can_create=false access rows (FK'd comparisons keep the
--      definitions; a type with no access rows is unrestricted, so deny
--      rows per group are the visibility gate).
--   2. New "Business" family — business_registration moves under it.
--   3. bank_account_registration moves under the banking family.

BEGIN;

-- A type with no access rows is unrestricted; any row makes the picker
-- gate on can_create grants. One can_create=false row on the all-groups
-- zero key therefore hides the type for every group while preserving
-- definitions for existing comparisons.
INSERT INTO comparison_type_access (type_id, user_group_id, can_create)
SELECT t.id, ug.id, false
FROM comparison_type_definitions t
CROSS JOIN user_groups ug
WHERE t.slug IN ('city-prices', 'city-transportation-prices')
ON CONFLICT (type_id, group_key) DO UPDATE SET can_create = false;

UPDATE comparison_type_definitions
SET config = config || '{"family": "business"}'::jsonb
WHERE slug = 'business_registration';

UPDATE comparison_type_definitions
SET config = config || '{"family": "banking"}'::jsonb
WHERE slug = 'bank_account_registration';

COMMIT;
