-- Rollback: Remove i18n keys added for wallet admin page.
DELETE FROM frontend_i18n WHERE key IN (
    'common.inactive',
    'common.adjusting',
    'app.wallet.edit_rule',
    'app.wallet.settings_saved',
    'app.wallet.error_delete_rules',
    'app.wallet.adjust_success',
    'app.wallet.user_not_found'
);
