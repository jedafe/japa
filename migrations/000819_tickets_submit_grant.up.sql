-- 000819_tickets_submit_grant.up.sql — §38: tickets.submit is
-- "default: all" per the spec, but the seed never granted the node to any
-- role — regular users got 403 on POST /tickets and (new) POST /appeals.
-- Grant it to every role that already holds tickets.view_own (Member,
-- Pro Member, Business Member — i.e. all self-service member tiers).
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions p ON p.key = 'tickets.submit'
JOIN permissions existing ON existing.id = rp.permission_id
  AND existing.key = 'tickets.view_own'
ON CONFLICT (role_id, permission_id) DO NOTHING;
