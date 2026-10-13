-- 000835_docs_portal_settings
-- Seed the docs-portal public settings so they are exposed via
-- GET /site/settings/public (is_public=true) and editable in Admin → Settings.
-- Code falls back to the catalog defaults when a row is absent/invalid, so
-- these seeds only materialize the defaults.

BEGIN;

INSERT INTO system_settings (key, value, is_public)
SELECT s.k, s.v::jsonb, true
FROM (VALUES
    ('docs.pinned_pages',        '[]'),
    ('docs.versions',            '[]'),
    ('docs.changelog_category',  '"Changelog"'),
    ('docs.changelog_new_days',  '14'),
    ('docs.help_center_url',     '""'),
    ('docs.support_url',         '""'),
    ('docs.repo_url',            '""')
) AS s(k, v)
WHERE NOT EXISTS (
    SELECT 1 FROM system_settings WHERE system_settings.key = s.k
);

COMMIT;
