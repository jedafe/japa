-- Remove the seeded super_admin user and related data.

-- 1. Remove group membership
DELETE FROM user_group_memberships
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND user_id = (
      SELECT id FROM users
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND email = 'jedafeglobal@gmail.com'
  );

-- 2. Remove user profile
DELETE FROM user_profiles
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND user_id = (
      SELECT id FROM users
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND email = 'jedafeglobal@gmail.com'
  );

-- 3. Remove the user
DELETE FROM users
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND email = 'jedafeglobal@gmail.com';
