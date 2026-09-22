-- 000693_intel_sources_view_permission.up.sql
-- Renumbered from 000435 — it collided with 000435_add_university_initials_trgm_index.
-- De-tenanted form: user_roles.tenant_id was dropped by 000668, so the original
-- tenant filter on the grant is gone. Tail-position body; databases that applied
-- the 000435 file skip this via the 000000 registry marker.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('intel.sources.view', 'Intel Sources View')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'member')
  AND p.key = 'intel.sources.view'
ON CONFLICT DO NOTHING;

COMMIT;
