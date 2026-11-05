BEGIN;

UPDATE comparison_items
SET item_ref = jsonb_set(item_ref, '{type}', '"organization"'::jsonb)
WHERE item_ref->>'type' = 'bank'
  AND item_ref->>'id' ~ '^[0-9a-fA-F-]{36}$'
  AND (item_ref->>'id')::uuid IN (SELECT id FROM banks);

COMMIT;
