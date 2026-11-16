-- 001174_federated_actor_key_rotation.up.sql
-- §126 M3a — actor key rotation: cooldown clock column, the new
-- federation.actors.manage RBAC node (admin rotate route), and the
-- cooldown setting seed so it is editable in Control → Settings →
-- Federation.
--
-- Ref: .jeos/sections/126-activitypub-federation.md (Key Rotation)
-- Ref: .jeos/conventions/permission-key-naming.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

ALTER TABLE federated_actors
    ADD COLUMN IF NOT EXISTS key_rotated_at timestamptz;

INSERT INTO permissions (key, description) VALUES
    ('federation.actors.manage', 'Mutate federated actors (admin): force-rotate signing keys')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin' AND p.key = 'federation.actors.manage'
ON CONFLICT DO NOTHING;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.key_rotation.cooldown_hours', to_jsonb(24), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
