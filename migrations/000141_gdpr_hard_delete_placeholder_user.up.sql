-- 000141_gdpr_hard_delete_placeholder_user.up.sql
-- Creates the Deleted User placeholder and seeds the setting for databases
-- that already ran 000137 before this fix. 

BEGIN;

-- Create the system "Deleted User" placeholder.
INSERT INTO users (id, tenant_id, email, username, status, profile_type) VALUES
    ('ffffffff-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000001',
     'deleted-user@system.japaconnect.local',
     'Deleted User',
     'deleted',
     'personal')
ON CONFLICT (id) DO NOTHING;

-- Seed the setting pointing at the placeholder user.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'gdpr.deleted_user_placeholder_uuid',
     '"ffffffff-0000-0000-0000-000000000001"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
