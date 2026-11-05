-- 001047 — JCE comparison_items.item_ref fixup after the bank entity split
-- (001045). Items stored as {"type":"organization","id":<bank uuid>} now
-- resolve against the banks table via the "bank" resolver, so retag them.
BEGIN;

UPDATE comparison_items
SET item_ref = jsonb_set(item_ref, '{type}', '"bank"'::jsonb)
WHERE item_ref->>'type' = 'organization'
  AND item_ref->>'id' ~ '^[0-9a-fA-F-]{36}$'
  AND (item_ref->>'id')::uuid IN (SELECT id FROM banks);

COMMIT;
