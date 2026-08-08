-- 000313_seed_mutual_aid_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('mutual_aid.create_need','mutual_aid.create_offer','mutual_aid.create_skill_swap','mutual_aid.moderate'));
DELETE FROM permissions WHERE key IN
    ('mutual_aid.create_need','mutual_aid.create_offer','mutual_aid.create_skill_swap','mutual_aid.moderate');
COMMIT;
