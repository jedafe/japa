-- Revert the system groups to the pre-000285 state: all at DEFAULT 0.
-- This restores the original (buggy) behavior where super_admin could not
-- create spaces due to the quota gate.

UPDATE user_groups
SET max_spaces_creatable = 0
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('super_admin', 'members', 'guest');
