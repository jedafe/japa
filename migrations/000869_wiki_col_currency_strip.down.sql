-- 000869_wiki_col_currency_strip.down.sql
-- Reverts only the default_source flag. The currency_code strip is
-- corrective — restoring "CAD" would resurrect the bug it fixed.
BEGIN;
UPDATE custom_fields f
SET options = f.options - 'default_source'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug LIKE 'wiki\_%'
  AND f.key = 'qf_currency';
COMMIT;
