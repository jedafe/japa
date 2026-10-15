-- 000863_agent_wiki_discovery_tools.down.sql
-- Removes the wiki discovery agent tools seeded by the up migration.

BEGIN;

DELETE FROM ai_tools
WHERE name IN (
    'wiki_page.list',
    'wiki_page.read',
    'wiki_page.revisions.list',
    'wiki.entity_fields.get'
);

COMMIT;
