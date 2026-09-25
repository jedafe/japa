-- 000452_seed_ai_provider_budget_rbac.down.sql
-- Reverts the granular AI provider/budget RBAC nodes.

BEGIN;

-- 1. Revoke grants from super_admin.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'ai.providers.manage', 'ai.providers.view', 'ai.budget.view', 'ai.budget.manage'
    )
);

-- 2. Remove the permission nodes.
DELETE FROM permissions WHERE key IN (
    'ai.providers.manage', 'ai.providers.view', 'ai.budget.view', 'ai.budget.manage'
);

COMMIT;
