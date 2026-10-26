-- 000936_verification_fields_guest_grants.up.sql
-- The §07 verification-request embed filters fields through
-- filterGuestViewableFields → custom_field_permissions.can_view for the
-- 'guest' user group (CF13). The 000933/000934 seeds never created grants,
-- so GET /forms/verification-request/embed returned zero fields — an
-- empty form. Grant can_view + can_edit to guest/members/super_admin on
-- every verification-request field (mirrors the 423-field seed pattern).

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, true
FROM custom_fields f
CROSS JOIN user_groups ug
WHERE f.group_id = '00000000-0000-0000-0000-00000000d001'
  AND ug.slug IN ('guest', 'members', 'super_admin')
ON CONFLICT (field_id, user_group_id) DO UPDATE SET can_view = true, can_edit = true;
