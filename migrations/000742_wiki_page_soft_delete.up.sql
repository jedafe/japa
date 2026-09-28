-- 000742_wiki_page_soft_delete.up.sql
-- Adds wiki_pages.deleted_at for the admin soft delete (§80) and seeds the
-- wiki.delete permission node. Soft delete also flips is_active=false so
-- every existing public reader (page GETs, destination counts, the
-- is_japabot_indexed generated column) excludes the row without a
-- schema-wide predicate change. Node grants follow the all-tenants pattern
-- from 000491 (admin + super_admin).
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
-- Ref: japaconnect-engineering/sections/03-rbac.md

BEGIN;

ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

INSERT INTO permissions (key, description)
VALUES ('wiki.delete', 'Delete wiki pages — soft delete and permanent removal (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'wiki.delete'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
