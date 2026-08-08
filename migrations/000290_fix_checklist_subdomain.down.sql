-- 000290_fix_checklist_subdomain.down.sql
-- Revert checklist.subdomain to the pre-correction value "visa".
-- This restores the incorrect state; only run if the correction needs to be
-- rolled back. The spec-correct value is "visa-checklist" (§70, ADR-070).
UPDATE system_settings
SET value = '"visa"'::jsonb,
    updated_at = now()
WHERE key = 'checklist.subdomain'
  AND value = '"visa-checklist"'::jsonb;
