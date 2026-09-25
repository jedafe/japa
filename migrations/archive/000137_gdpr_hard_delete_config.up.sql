-- 000137_gdpr_hard_delete_config.up.sql
-- Seeds GDPR hard-delete configuration settings per ADR-054.
-- gdpr.hard_delete_enabled controls whether physical deletion follows anonymization.
-- gdpr.retention_days sets optional delay before hard delete (0 = immediate).
-- gdpr.deleted_user_placeholder_uuid points to a system user that inherits
-- content authorship after an author is permanently erased. 

BEGIN;

-- Create a system "Deleted User" placeholder that inherits content authorship
-- after a user is physically erased. The FK on contents.author_id requires a
-- valid user row to satisfy the constraint during ReassignContentAuthor.
INSERT INTO users (id, tenant_id, email, username, status, profile_type) VALUES
    ('ffffffff-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000001',
     'deleted-user@system.japaconnect.local',
     'Deleted User',
     'deleted',
     'personal')
ON CONFLICT (id) DO NOTHING;

-- Seed hard-delete configuration.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'gdpr.hard_delete_enabled', '"false"'),
    ('00000000-0000-0000-0000-000000000001', 'gdpr.retention_days', '"0"'),
    ('00000000-0000-0000-0000-000000000001', 'gdpr.deleted_user_placeholder_uuid',
     '"ffffffff-0000-0000-0000-000000000001"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
