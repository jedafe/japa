-- 000760_wiki_destinations_page_limit_seed.up.sql
-- §80 Reader UX: destinations directory default page size. Code falls back
-- to 24 via GetIntOr when absent; the seed makes the key admin-editable
-- (system-settings-catalog convention: keys with defaults are seeded).
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('wiki.destinations_page_limit', to_jsonb(24))
ON CONFLICT (key) DO NOTHING;

COMMIT;
