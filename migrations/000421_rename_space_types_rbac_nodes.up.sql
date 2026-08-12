-- BUG-006: Rename space_types.* RBAC nodes to spaces.types.* to follow
-- the {domain}.{subdomain?}.{action} naming convention (permission-key-naming.md).
-- The permissions table uses a text `key` column referenced by ID from
-- role_permissions, so a simple UPDATE is sufficient — no FK changes needed.
-- The old keys were seeded in 000099_seed_data.up.sql and re-seeded in
-- 000109_rbac_permission_reconciliation.up.sql.

UPDATE permissions SET key = 'spaces.types.create' WHERE key = 'space_types.create';
UPDATE permissions SET key = 'spaces.types.edit'   WHERE key = 'space_types.edit';
UPDATE permissions SET key = 'spaces.types.delete' WHERE key = 'space_types.delete';
