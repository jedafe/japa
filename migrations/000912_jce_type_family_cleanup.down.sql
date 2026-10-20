-- 000912_jce_type_family_cleanup.down.sql
BEGIN;

DELETE FROM comparison_type_access cta
USING comparison_type_definitions t
WHERE cta.type_id = t.id
  AND t.slug IN ('city-prices', 'city-transportation-prices');

UPDATE comparison_type_definitions
SET config = config - 'family'
WHERE slug IN ('business_registration', 'bank_account_registration');

COMMIT;
