-- 000836_docs_link_settings
-- Seed the docs-portal link/resource settings managed by the new admin
-- repeaters. help_links / contribute_links are composed from the legacy
-- docs.*_url settings so existing configuration carries over; they are
-- only inserted when the composed array is non-empty — an absent key keeps
-- the frontend's built-in fallback defaults (a stored [] explicitly hides
-- the links). dev_resources seeds the built-in defaults.

BEGIN;

-- 'Need help' links: Help Center (only when configured) + Contact Support.
INSERT INTO system_settings (key, value, is_public)
SELECT 'docs.help_links', arr.v, true
FROM (
    SELECT jsonb_agg(item) AS v
    FROM (
        SELECT jsonb_build_object('label', 'Visit Help Center', 'url', u) AS item
        FROM (SELECT value #>> '{}' AS u FROM system_settings WHERE key = 'docs.help_center_url') x
        WHERE u <> ''
        UNION ALL
        SELECT jsonb_build_object('label', 'Contact Support', 'url', u)
        FROM (SELECT value #>> '{}' AS u FROM system_settings WHERE key = 'docs.support_url') x
        WHERE u <> ''
    ) s
) arr
WHERE arr.v IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'docs.help_links');

-- 'Contribute to Docs' links: suggest changes + report an issue off the repo URL.
INSERT INTO system_settings (key, value, is_public)
SELECT 'docs.contribute_links', arr.v, true
FROM (
    SELECT jsonb_agg(item) AS v
    FROM (
        SELECT jsonb_build_object('label', 'Suggest changes', 'url', u) AS item
        FROM (SELECT rtrim(value #>> '{}', '/') AS u FROM system_settings WHERE key = 'docs.repo_url') x
        WHERE u <> ''
        UNION ALL
        SELECT jsonb_build_object('label', 'Report an issue', 'url', u || '/issues')
        FROM (SELECT rtrim(value #>> '{}', '/') AS u FROM system_settings WHERE key = 'docs.repo_url') x
        WHERE u <> ''
    ) s
) arr
WHERE arr.v IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'docs.contribute_links');

-- Developer resources: materialize the built-in defaults so admins edit them.
INSERT INTO system_settings (key, value, is_public)
SELECT 'docs.dev_resources',
       '[
         {"description": "Try requests interactively", "icon": "play", "label": "API Playground", "url": "/docs/api/playground"},
         {"description": "Create and manage access tokens", "icon": "key", "label": "API Keys", "url": "/docs/api/keys"},
         {"description": "Download the OpenAPI schema", "icon": "file", "label": "OpenAPI Spec", "url": "/docs/openapi.json"}
       ]'::jsonb,
       true
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'docs.dev_resources');

COMMIT;
