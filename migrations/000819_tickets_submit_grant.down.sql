-- 000819_tickets_submit_grant.down.sql — revoke the tickets.submit grant
-- from the member-tier roles (restores the pre-migration state where no
-- role held the node).
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'tickets.submit')
  AND role_id IN (
      SELECT ur.id FROM user_roles ur
      WHERE ur.name IN ('Member', 'Pro Member', 'Business Member')
  );
