-- 000235_seed_admin_comparisons_i18n.down.sql
-- Revert the admin /comparisons i18n keys added in 000235.

DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND "key" IN (
    'comparisons.col_title', 'comparisons.col_slug', 'comparisons.col_visibility',
    'comparisons.col_status', 'comparisons.col_created',
    'comparisons.vis_public', 'comparisons.vis_members', 'comparisons.vis_private',
    'comparisons.create', 'comparisons.edit', 'comparisons.view', 'comparisons.body',
    'comparisons.type', 'comparisons.type_none', 'comparisons.delete_confirm'
  );
