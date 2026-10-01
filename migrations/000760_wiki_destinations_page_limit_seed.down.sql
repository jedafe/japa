-- 000760_wiki_destinations_page_limit_seed.down.sql — remove the seeded setting.

BEGIN;

DELETE FROM system_settings WHERE key = 'wiki.destinations_page_limit';

COMMIT;
