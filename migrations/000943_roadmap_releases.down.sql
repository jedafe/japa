-- 000943_roadmap_releases.down.sql
-- Remove the versioned-releases tables, the scope-map setting, and the
-- eight release agent tools. The reused roadmap.* permission nodes are
-- left in place — they predate this migration and gate the REST routes.

BEGIN;

DELETE FROM ai_tools WHERE name IN (
    'roadmap.releases.list',
    'roadmap.release.get',
    'roadmap.release.create',
    'roadmap.release.update',
    'roadmap.release.set_status',
    'roadmap.release.attach_items',
    'roadmap.entries.list',
    'roadmap.entries.ingest'
);

DELETE FROM system_settings WHERE key = 'roadmap.commit_scope_paths';

DROP TABLE roadmap_release_entries;
ALTER TABLE roadmap_items DROP COLUMN release_id;
DROP TABLE roadmap_releases;

COMMIT;
