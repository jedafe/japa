-- 000257_mobile_push_devices.up.sql
-- Mobile Push / Device Tokens (Section 77, ADR-074).
-- Creates the device_tokens table for FCM/APNs token registration,
-- and seeds push.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
--      japaconnect-engineering/adr/ADR-074-screen-contract.md

BEGIN;

-- =====================================================================
-- device_tokens — per-user device registration for push notifications.
-- A user can have multiple devices (phone, tablet). Token is invalidated
-- when FCM/APNs reports it as invalid (is_active = false).
-- =====================================================================
CREATE TABLE IF NOT EXISTS device_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id    text NOT NULL,          -- client-generated unique device ID
    token        text NOT NULL,           -- FCM or APNs token
    platform     text NOT NULL CHECK (platform IN ('ios','android','web')),
    app_version  text,
    is_active    boolean NOT NULL DEFAULT true,
    last_used_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user
    ON device_tokens (tenant_id, user_id, is_active);

CREATE INDEX IF NOT EXISTS idx_device_tokens_token
    ON device_tokens (token)
    WHERE is_active = true;

-- =====================================================================
-- system_settings — 5 push.* keys (Section 77)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'push.fcm_server_key',    '""'),
    ('00000000-0000-0000-0000-000000000001', 'push.apns_key_id',       '""'),
    ('00000000-0000-0000-0000-000000000001', 'push.apns_team_id',      '""'),
    ('00000000-0000-0000-0000-000000000001', 'push.apns_private_key',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'push.enabled',           '"false"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 push.* nodes (Section 77)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('push.manage',  'Configure FCM/APNs credentials, send test pushes (admin)'),
    ('push.receive', 'Receive push notifications (all authenticated users)')
ON CONFLICT (key) DO NOTHING;

-- Grant push.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'push.manage'
ON CONFLICT DO NOTHING;

-- Grant push.receive to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'push.receive'
ON CONFLICT DO NOTHING;

COMMIT;
