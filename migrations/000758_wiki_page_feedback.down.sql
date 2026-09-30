-- 000758_wiki_page_feedback (down)

BEGIN;

DROP TABLE IF EXISTS wiki_page_feedback;

DELETE FROM system_settings WHERE key = 'wiki.destinations_page_limit';

COMMIT;
