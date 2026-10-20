-- 000905_wiki_scholarship_parent_fix.down.sql
-- Reverses 000905: restores the scholarship → university parent edge
-- (unsatisfiable without a university space FK, but kept here for an
-- exact down of the original seed).

BEGIN;

INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id)
VALUES ('00000000-0000-0000-0000-00000000007b', '00000000-0000-0000-0000-000000000064')
ON CONFLICT DO NOTHING;

COMMIT;
