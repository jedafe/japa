-- 001042_normalize_org_type_options.down.sql
-- Restores the option-list shape (the "banks" duplicate and the capital-A
-- "Agency" value). Organization row remapping is intentionally not reversed —
-- it is a data normalization and the original value of a remapped row cannot
-- be distinguished from rows that were always 'bank'/'agency'.
BEGIN;

UPDATE custom_fields
SET options = (
    SELECT jsonb_agg(opt)
    FROM (
        SELECT CASE WHEN opt->>'value' = 'agency'
                    THEN jsonb_set(opt, '{value}', '"Agency"')
                    ELSE opt
               END AS opt
        FROM jsonb_array_elements(options) AS opt
        UNION ALL
        SELECT '{"label": "Banks", "value": "banks"}'::jsonb
    ) AS merged
)
WHERE key = 'org_type'
  AND options IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(options) AS opt WHERE opt->>'value' = 'banks');

COMMIT;
