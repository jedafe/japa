-- Seed the initial super_admin user with credentials.
-- Email: jedafeglobal@gmail.com
-- Username: jedafeglobal (derived from email prefix)
-- Password: bcrypt hash of provided password (cost 12)
-- Uses ON CONFLICT for idempotency.

-- 1. Insert the super_admin user
INSERT INTO users (tenant_id, email, username, password_hash, profile_type, status, email_verified_at)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'jedafeglobal@gmail.com',
    'jedafeglobal',
    '$2a$12$4jeCRM3aq48/gvkFFvDkeOdylJbEQB8S8Nmii0OxWupn8DEWog3x6',
    'personal',
    'active',
    now()
)
ON CONFLICT (tenant_id, email) DO NOTHING;

-- 2. Create user profile for the super_admin user
INSERT INTO user_profiles (tenant_id, user_id, display_name)
SELECT '00000000-0000-0000-0000-000000000001',
       id,
       'Super Admin'
FROM users
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND email = 'jedafeglobal@gmail.com'
ON CONFLICT (tenant_id, user_id) DO NOTHING;

-- 3. Assign the user to the super_admin group
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
SELECT '00000000-0000-0000-0000-000000000001',
       u.id,
       '00000000-0000-0000-0000-000000000010'
FROM users u
WHERE u.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND u.email = 'jedafeglobal@gmail.com'
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;
