-- 000905_wiki_scholarship_parent_fix.up.sql
-- Fix-forward review correction (post-000900): scholarship was parented to
-- university + country, but the entity-children resolution keys on
-- scholarships.country_space_id — the university edge can never produce
-- children (no university space FK on scholarships; the `university`
-- column is free text). Dead config: a university index would silently
-- show an empty scholarship section. Drop the edge; restore it when a real
-- scholarships.university_space_id FK exists and the children dispatch
-- becomes host-type-aware.

BEGIN;

DELETE FROM wiki_type_parents
WHERE space_type_id = '00000000-0000-0000-0000-00000000007b'   -- scholarship
  AND parent_space_type_id = '00000000-0000-0000-0000-000000000064'; -- university

COMMIT;
