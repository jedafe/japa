-- 000835_docs_portal_settings (down)

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'docs.pinned_pages',
    'docs.versions',
    'docs.changelog_category',
    'docs.changelog_new_days',
    'docs.help_center_url',
    'docs.support_url',
    'docs.repo_url'
);

COMMIT;
