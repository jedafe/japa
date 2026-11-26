-- 001231_seed_dm_member_permissions.up.sql
-- Grants the dm.* nodes that were seeded but never assigned to any role.
-- 000735_baseline_seed granted dm.send + dm.request to member/pro_member/
-- business_member but skipped dm.view; archive 000144 targeted
-- 'member_role' (a slug that does not exist), so dm.delete silently
-- granted zero rows; dm.manage_privacy was never granted at all.
-- Result: every member-tier user was 403'd on GET /dm/conversations,
-- /dm/privacy, /dm/requests and /read despite legitimately holding
-- dm.send/dm.request (server.go wires dm.view on all read routes —
-- BUG-016). super_admin is covered by the "*" wildcard.
--
-- Ref: internal/server/server.go (dm.* route gates)

BEGIN;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, now()
FROM user_roles r
JOIN permissions p ON p.key IN ('dm.view', 'dm.delete', 'dm.manage_privacy')
WHERE r.slug IN ('member', 'pro_member', 'business_member')
ON CONFLICT DO NOTHING;

COMMIT;
