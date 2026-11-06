BEGIN;
DELETE FROM wiki_entity_kinds WHERE entity_kind IN ('product', 'provider', 'deal');
COMMIT;
