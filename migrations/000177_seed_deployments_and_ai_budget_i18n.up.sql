-- 000177_seed_deployments_and_ai_budget_i18n.up.sql
-- Seeds i18n keys for:
--   * Deployments page (nav + page content) — new page added to Control Panel
--   * AI Budget edit dialog (edit_threshold, edit_threshold_desc, invalid_threshold, save_error)
--   * Security scan Resolve row action label
-- Idempotent: ON CONFLICT DO UPDATE so re-running corrects stale values.

INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    -- nav.deployments (sidebar label + tooltip)
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.deployments', 'Deployments'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'nav.deployments', 'Déploiements'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'nav.deployments_tooltip', 'Deployment version history'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'nav.deployments_tooltip', 'Historique des versions déployées'),

    -- deployments page content
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.title', 'Deployments'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.title', 'Déploiements'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.description', 'Current deployment version and history'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.description', 'Version actuelle et historique des déploiements'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.version', 'Version'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.version', 'Version'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.deployed_at', 'Deployed At'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.deployed_at', 'Déployé le'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.unavailable', 'Deployment information is currently unavailable.'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.unavailable', 'Les informations de déploiement sont actuellement indisponibles.'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'deployments.no_data', 'No deployment records found.'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'deployments.no_data', 'Aucun enregistrement de déploiement trouvé.'),

    -- ai_budget edit dialog
    ('00000000-0000-0000-0000-000000000001', 'en', 'ai_budget.edit_threshold', 'Edit Threshold'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'ai_budget.edit_threshold', 'Modifier le seuil'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'ai_budget.edit_threshold_desc', 'Set the monthly AI spend alert threshold in USD.'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'ai_budget.edit_threshold_desc', 'Définir le seuil d''alerte mensuel des dépenses IA en USD.'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'ai_budget.invalid_threshold', 'Threshold must be a non-negative number.'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'ai_budget.invalid_threshold', 'Le seuil doit être un nombre non négatif.'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'ai_budget.save_error', 'Failed to update budget threshold.'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'ai_budget.save_error', 'Échec de la mise à jour du seuil du budget.'),

    -- security resolve row action
    ('00000000-0000-0000-0000-000000000001', 'en', 'security.resolve', 'Resolve'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'security.resolve', 'Résoudre')
ON CONFLICT (tenant_id, locale, "key") DO UPDATE SET value = EXCLUDED.value, updated_at = now();
