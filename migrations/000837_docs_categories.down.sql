-- 000837_docs_categories down: drop the seeded managed category catalog.
BEGIN;

DELETE FROM system_settings WHERE key = 'docs.categories';

COMMIT;
