-- 000820_appeal_review_grant.down.sql — revoke the moderation.appeal.review
-- grant from the super_admin roles (restores the pre-migration state where no
-- role held the node). The permission row itself is left in place.

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'moderation.appeal.review')
  AND role_id IN (
      SELECT ur.id FROM user_roles ur
      WHERE ur.slug IN ('super_admin', 'super_admin_role')
  );
