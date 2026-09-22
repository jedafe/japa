-- 000405_seed_ai_agent_phase9_tools.down.sql
-- Renumbered from 000400 — see the .up file for the collision note.
-- Reverts the Phase 9 extended tool catalog seed.
-- Deletes only the Phase 9 tool rows — Phase 4/7/8 rows seeded by earlier
-- migrations are untouched.

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN (
    -- Spaces (9.01)
    'spaces.find', 'spaces.read', 'spaces.list', 'spaces.create', 'spaces.update',
    -- Learning (9.02)
    'learn.tracks.list', 'learn.tracks.create', 'learn.tracks.update',
    'learn.lessons.list', 'learn.lessons.create', 'learn.lessons.update',
    'learn.lessons.reorder',
    -- Docs (9.03)
    'docs.list', 'docs.read', 'docs.create', 'docs.update', 'docs.organize',
    -- Tools app (9.04)
    'tools.list', 'tools.read', 'tools.create', 'tools.update', 'tools.assign_geo',
    -- Menus (9.05)
    'menu.list', 'menu.create', 'menu.update', 'menu.delete',
    -- User (9.06)
    'user.read_profile', 'user.update_profile', 'user.read_own_fields',
    -- Custom field definitions (9.07)
    'custom_fields.create', 'custom_fields.update',
    'custom_field_groups.list', 'custom_field_groups.create',
    -- Content lifecycle (9.08)
    'content.delete', 'content.list', 'content.publish'
  );

COMMIT;
