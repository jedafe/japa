-- Table name corrected from a nonexistent "i18n_keys" to the real
-- translations table, matching the up.sql fix (see
-- migrations/000046_i18n.sql for the table definition).
DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND locale IN ('en', 'fr')
  AND "key" = 'api_keys.tier_basic';
