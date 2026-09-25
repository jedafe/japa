-- 000177_seed_deployments_and_ai_budget_i18n.down.sql
-- Removes the i18n keys seeded by the companion up migration.

DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND "key" IN (
    'nav.deployments',
    'nav.deployments_tooltip',
    'deployments.title',
    'deployments.description',
    'deployments.version',
    'deployments.deployed_at',
    'deployments.unavailable',
    'deployments.no_data',
    'ai_budget.edit_threshold',
    'ai_budget.edit_threshold_desc',
    'ai_budget.invalid_threshold',
    'ai_budget.save_error',
    'security.resolve'
  );
