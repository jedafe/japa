-- 001034_drop_admin_settings_manage.down.sql

BEGIN;

INSERT INTO permissions (id, key, description, created_at) VALUES
    (gen_random_uuid(), 'admin.settings.manage', 'Manage platform settings', now())
ON CONFLICT (key) DO NOTHING;

COMMIT;
