-- 000299_seed_story_rbac_nodes.down.sql
-- Reverses 000299_seed_story_rbac_nodes.up.sql — removes the six story.*
-- permission nodes and all their role_permissions grants.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('story.create', 'story.edit_own', 'story.edit_any',
                  'story.delete_own', 'story.delete_any', 'story.verify')
);

DELETE FROM permissions
WHERE key IN ('story.create', 'story.edit_own', 'story.edit_any',
              'story.delete_own', 'story.delete_any', 'story.verify');

COMMIT;
