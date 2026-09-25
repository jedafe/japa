-- Add description column to user_groups (Section 03 RBAC schema amendment).
-- The spec defines user_groups.description but the original migration 000006
-- omitted it. The CreateGroup/UpdateGroup handlers already accept a
-- description field but could not persist it.
ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS description text;
