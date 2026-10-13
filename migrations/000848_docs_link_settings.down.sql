-- 000836_docs_link_settings (down)
-- Remove the repeater-driven link/resource settings; the legacy docs.*_url
-- rows are untouched so the frontend fallback keeps working.

BEGIN;

DELETE FROM system_settings
WHERE key IN ('docs.help_links', 'docs.contribute_links', 'docs.dev_resources');

COMMIT;
