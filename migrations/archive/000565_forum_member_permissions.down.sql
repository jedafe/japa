BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
  SELECT id FROM user_roles
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
    AND slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
)
  AND permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
      'forum.poll.create',
      'forum.tag.create',
      'forum.tag.assign',
      'forum.thread.edit_own',
      'forum.thread.edit_any',
      'forum.thread.delete_own',
      'forum.thread.delete_any',
      'forum.reply.edit_own',
      'forum.reply.edit_any',
      'forum.reply.delete_own',
      'forum.reply.delete_any',
      'forum.thread.solve',
      'forum.tag.edit',
      'forum.tag.moderate'
    )
  );

COMMIT;
