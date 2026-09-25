-- 000693_intel_sources_view_permission.down.sql
-- Renumbered from 000435 (duplicate version prefix). See the .up file.
-- 000435_intel_sources_view_permission.down.sql
-- Reverses 000435: removes the intel.sources.view grant and permission node.
-- Safe to run at any time; the node is intelligence-specific.

BEGIN;

-- 1. Remove role_permissions grants for intel.sources.view.
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key = 'intel.sources.view';

-- 2. Remove the permission node itself.
DELETE FROM permissions WHERE key = 'intel.sources.view';

COMMIT;
