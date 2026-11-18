-- 001207_remove_checklist_domain_settings.up.sql
-- §70/audit BUG-022: checklist.subdomain + checklist.custom_domain are dead
-- config — nothing reads them (the tools app consolidated into the main app;
-- the visa-checklist. CORS host pattern is hardcoded in origin.go). Removing
-- them drops two editable-but-inert rows from Control → Settings.

BEGIN;

DELETE FROM system_settings
WHERE key IN ('checklist.subdomain', 'checklist.custom_domain');

COMMIT;
