-- 000902_jce_wiki_family_types_seed.down.sql
-- Reverses 000902: removes the seeded family comparison types.

BEGIN;

DELETE FROM comparison_type_definitions
WHERE slug IN (
    'scholarships',
    'visa-routes',
    'student-visas',
    'work-visas',
    'family-visas',
    'tourist-visas',
    'business-visas',
    'pr-pathways'
);

COMMIT;
