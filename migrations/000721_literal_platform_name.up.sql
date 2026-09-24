-- 000721_literal_platform_name.up.sql
-- Seed the canonical Platform_Name token (the site/tenant display name) and
-- point Tenant_Name at the same variable. Tenant_Name is kept as an alias
-- for backward compatibility with existing content.
--
-- NOTE: written against the de-tenanted schema — 000668_final_de_tenant
-- dropped template_literals.tenant_id and the unique index is on (key).

INSERT INTO template_literals (key, kind, value, fallback, label, description, category, is_system)
VALUES
    ('Platform_Name', 'expression', 'Platform_Name', '', 'Platform Name', 'The platform display name (site.name).', 'tenant', true)
ON CONFLICT DO NOTHING;

UPDATE template_literals
SET value = 'Platform_Name', updated_at = now()
WHERE key = 'Tenant_Name'
  AND value = 'Tenant_Name'
  AND deleted_at IS NULL;
