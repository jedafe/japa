-- Normalize comparison-type slugs to hyphenated form, matching the rest
-- of the catalog: cost_of_living → cost-of-living and the three
-- *_registration types. allowed_nested_types arrays that referenced the
-- old slugs are rewritten first (item_ref_types hold resolver keys —
-- unaffected). Row ids are unchanged, so comparisons keep their type.

BEGIN;

UPDATE comparison_type_definitions td
SET config = jsonb_set(td.config, '{allowed_nested_types}', (
    SELECT COALESCE(jsonb_agg(COALESCE(r.new, e.el) ORDER BY e.ord), '[]'::jsonb)
    FROM jsonb_array_elements_text(td.config->'allowed_nested_types')
         WITH ORDINALITY AS e(el, ord)
    LEFT JOIN (VALUES
        ('cost_of_living',                'cost-of-living'),
        ('business_registration',         'business-registration'),
        ('bank_account_registration',     'bank-account-registration'),
        ('residents_permit_requirements', 'residents-permit-requirements')
    ) AS r(old, new) ON r.old = e.el
))
WHERE td.config ? 'allowed_nested_types';

UPDATE comparison_type_definitions SET slug = 'cost-of-living'                 WHERE slug = 'cost_of_living';
UPDATE comparison_type_definitions SET slug = 'business-registration'          WHERE slug = 'business_registration';
UPDATE comparison_type_definitions SET slug = 'bank-account-registration'      WHERE slug = 'bank_account_registration';
UPDATE comparison_type_definitions SET slug = 'residents-permit-requirements'  WHERE slug = 'residents_permit_requirements';

COMMIT;
