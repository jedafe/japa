-- 000739_wiki_types_categories.down.sql
-- Reverse the wiki type/category template tables.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

DROP TABLE IF EXISTS wiki_type_categories;
DROP TABLE IF EXISTS wiki_type_parents;
DROP TABLE IF EXISTS wiki_type_configs;

COMMIT;
