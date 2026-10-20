-- 000907_embassies_table.down.sql
-- Reverses 000907: restores the monitored_embassies binding and drops the
-- dedicated table. Embassies rows created since are lost with the table —
-- the polling table is untouched so monitoring keeps working.

BEGIN;

UPDATE wiki_entity_kinds
SET source_table = 'monitored_embassies'
WHERE entity_kind = 'embassy';

DROP TABLE IF EXISTS embassies;

COMMIT;
