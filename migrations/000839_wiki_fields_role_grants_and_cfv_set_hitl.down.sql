-- 000839_wiki_fields_role_grants_and_cfv_set_hitl.down.sql

BEGIN;

DELETE FROM role_permissions rp
USING user_roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.slug IN ('pro_member', 'business_member')
  AND p.name IN ('wiki.fields.edit', 'wiki.fields.propose');

UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'false'::jsonb),
    updated_at = now()
WHERE name = 'custom_field_values.set';

COMMIT;
