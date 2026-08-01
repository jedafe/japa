-- 000230_ai_rbac_reconciliation.up.sql
-- Section 20 (AI Core) + Section 03 (RBAC) + ADR-025 conformance fix.
--
-- Two problems addressed:
--   1. BUG-001 (critical): migration 000109 bulk-granted super_admin EVERY
--      permission node, including all `ai.context.*` nodes. Section 20 amendment
--      and the Bug Hunter's Guide §19 mandate: "all ai.context.* nodes default
--      OFF, including for super_admin — every node granted manually." This
--      revokes every `ai.context.*` grant from super_admin_role so the policy
--      holds. (Admins still get `ai.context.admin_view` / `ai.context.admin_delete_entry`
--      via the separate admin grants below — those are admin-capability nodes,
--      not the user-context-feed nodes that must stay off by default.)
--   2. BUG-002 / BUG-003 / BUG-011: the routes `/ai/chat` (requires
--      `ai.context.use`) and `/ai/assist/improve` (requires `ai.editor.assist`)
--      enforce nodes that were never seeded. Seed both nodes here. Per §19,
--      `ai.context.use` is an `ai.context.*` node and therefore stays default
--      OFF (not granted to super_admin). `ai.editor.assist` is an editor
--      capability, not a context-feed node, so it IS granted to super_admin.
--
-- Idempotent; safe to re-run.

BEGIN;

-- 1. Seed the two missing AI permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('ai.context.use', 'Use JapaBot conversational AI (ai.context.* family, default OFF incl. super_admin)'),
    ('ai.editor.assist', 'Use AI editor assist (improve/summarize/translate/generate-tags/generate-og)')
ON CONFLICT (key) DO NOTHING;

-- 2. Revoke every ai.context.* node from super_admin_role so the §19
--    "default OFF including for super_admin" policy holds. admin_view and
--    admin_delete_entry are admin capabilities, not context-feed nodes, so
--    they are intentionally re-granted in step 3.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND rp.permission_id = p.id
  AND p.key LIKE 'ai.context.%';

-- 3. Re-grant the two admin-capability ai.context.* nodes to super_admin.
--    These are NOT user-context-feed nodes; they gate admin visibility of
--    Honcho memory (ADR-027) and must remain available to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('ai.context.admin_view', 'ai.context.admin_delete_entry')
ON CONFLICT DO NOTHING;

-- 4. Grant ai.editor.assist to super_admin (editor capability, not a
--    context-feed node, so it is not subject to the §19 default-off rule).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'ai.editor.assist'
ON CONFLICT DO NOTHING;

COMMIT;
