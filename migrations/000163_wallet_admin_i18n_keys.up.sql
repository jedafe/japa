-- Migration 000163: Add missing i18n keys for wallet admin page.
-- Ref: Bug Hunt — admin wallet page i18n compliance (§7).
INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    -- common.inactive
    ('00000000-0000-0000-0000-000000000001', 'en', 'common.inactive', 'Inactive'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'common.inactive', 'Inactif'),
    -- common.adjusting
    ('00000000-0000-0000-0000-000000000001', 'en', 'common.adjusting', 'Adjusting...'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'common.adjusting', 'Ajustement...'),
    -- app.wallet.edit_rule
    ('00000000-0000-0000-0000-000000000001', 'en', 'app.wallet.edit_rule', 'Edit Activity Rule'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'app.wallet.edit_rule', 'Modifier la règle d''activité'),
    -- app.wallet.settings_saved
    ('00000000-0000-0000-0000-000000000001', 'en', 'app.wallet.settings_saved', 'Settings saved successfully'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'app.wallet.settings_saved', 'Paramètres enregistrés avec succès'),
    -- app.wallet.error_delete_rules
    ('00000000-0000-0000-0000-000000000001', 'en', 'app.wallet.error_delete_rules', 'Failed to delete selected rules'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'app.wallet.error_delete_rules', 'Échec de la suppression des règles sélectionnées'),
    -- app.wallet.adjust_success
    ('00000000-0000-0000-0000-000000000001', 'en', 'app.wallet.adjust_success', 'Adjusted {type} by {amount}'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'app.wallet.adjust_success', 'Ajusté {type} de {amount}'),
-- app.wallet.user_not_found
    ('00000000-0000-0000-0000-000000000001', 'en', 'app.wallet.user_not_found', 'User not found'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'app.wallet.user_not_found', 'Utilisateur non trouvé')
ON CONFLICT (tenant_id, locale, key) DO NOTHING;
