-- 000317_seed_japa_stage_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('profile.japa_stage.edit','profile.japa_stage.view'));
DELETE FROM permissions WHERE key IN
    ('profile.japa_stage.edit','profile.japa_stage.view');
COMMIT;
