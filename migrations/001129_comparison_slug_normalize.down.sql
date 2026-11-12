BEGIN;

UPDATE comparison_type_definitions td
SET config = jsonb_set(td.config, '{allowed_nested_types}', (
    SELECT COALESCE(jsonb_agg(COALESCE(r.new, e.el) ORDER BY e.ord), '[]'::jsonb)
    FROM jsonb_array_elements_text(td.config->'allowed_nested_types')
         WITH ORDINALITY AS e(el, ord)
    LEFT JOIN (VALUES
        ('cost-of-living',                'cost_of_living'),
        ('business-registration',         'business_registration'),
        ('bank-account-registration',     'bank_account_registration'),
        ('residents-permit-requirements', 'residents_permit_requirements')
    ) AS r(old, new) ON r.old = e.el
))
WHERE td.config ? 'allowed_nested_types';

UPDATE comparison_type_definitions SET slug = 'cost_of_living'                 WHERE slug = 'cost-of-living';
UPDATE comparison_type_definitions SET slug = 'business_registration'          WHERE slug = 'business-registration';
UPDATE comparison_type_definitions SET slug = 'bank_account_registration'      WHERE slug = 'bank-account-registration';
UPDATE comparison_type_definitions SET slug = 'residents_permit_requirements'  WHERE slug = 'residents-permit-requirements';

COMMIT;
