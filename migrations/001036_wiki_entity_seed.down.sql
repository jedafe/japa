-- 001036_wiki_entity_seed.down.sql
BEGIN;
DELETE FROM organizations WHERE org_type='bank' AND slug LIKE 'bank-%';
DELETE FROM embassies WHERE slug LIKE 'nigeria-mission-%' OR slug LIKE 'mission-%-in-nigeria';
DELETE FROM scholarships WHERE source_type='official' AND trust_level=5 AND confidence_overall=0.9;
COMMIT;
