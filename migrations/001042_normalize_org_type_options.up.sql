-- 001042_normalize_org_type_options.up.sql
-- §06/§07: the org_type select custom field accumulated two bad options —
-- a capital-A "Agency" value (baseline seed) and a duplicate "banks" value
-- alongside the canonical "bank". The /org directory renders one filter chip
-- per option, so "Bank" and "Banks" appear as separate filters. Normalize
-- the value to lowercase, drop the duplicate, and remap the org rows that
-- carry the legacy values.
BEGIN;

UPDATE custom_fields
SET options = (
    SELECT jsonb_agg(
        CASE WHEN opt->>'value' = 'Agency'
             THEN jsonb_set(opt, '{value}', '"agency"')
             ELSE opt
        END
    )
    FROM jsonb_array_elements(options) AS opt
    WHERE opt->>'value' <> 'banks'
)
WHERE key = 'org_type'
  AND options IS NOT NULL;

UPDATE organizations SET org_type = 'bank'   WHERE org_type = 'banks';
UPDATE organizations SET org_type = 'agency' WHERE org_type = 'Agency';

COMMIT;
