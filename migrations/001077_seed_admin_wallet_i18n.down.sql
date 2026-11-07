-- 001077_seed_admin_wallet_i18n.down
-- Removes the seeded admin wallet i18n keys. The pre-existing (incorrect)
-- 'Description' value of app.wallet.description is intentionally not
-- restored — restoring a bug is worse than keeping the fix.

BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'app.wallet.cap_unlimited_hint', 'app.wallet.admin_note',
    'app.wallet.admin_note_ph', 'app.wallet.approve', 'app.wallet.cap_none',
    'app.wallet.cap_per_day', 'app.wallet.cap_per_week', 'app.wallet.currency',
    'app.wallet.edit_rule', 'app.wallet.error_delete_packs', 'app.wallet.error_delete_rules',
    'app.wallet.error_load_packs', 'app.wallet.error_load_purchases',
    'app.wallet.error_load_withdrawals', 'app.wallet.error_process', 'app.wallet.fee',
    'app.wallet.loading_feature', 'app.wallet.net_amount', 'app.wallet.payout_provider',
    'app.wallet.provider_ref', 'app.wallet.rate', 'app.wallet.reject',
    'app.wallet.settings_saved', 'app.wallet.toggle', 'app.wallet.user_id',
    'app.wallet.user_not_found', 'app.wallet.withdraw_feature_disabled',
    'app.wallet.withdrawals_desc', 'app.wallet.withdrawals_title',
    'app.wallet.step_up_title_approve', 'app.wallet.step_up_title_reject',
    'app.wallet.step_up_help', 'app.wallet.step_up_password', 'app.wallet.step_up_code',
    'app.wallet.step_up_required', 'app.wallet.step_up_failed', 'app.wallet.col_date',
    'app.wallet.anonymous', 'app.wallet.ph_uuid', 'app.wallet.empty_purchases',
    'app.wallet.status_pending', 'app.wallet.status_pending_approval',
    'app.wallet.status_approved', 'app.wallet.status_processing',
    'app.wallet.status_completed', 'app.wallet.status_failed',
    'app.wallet.status_rejected', 'app.wallet.status_cancelled',
    'common.id', 'nav.withdrawals', 'nav.credit_purchases', 'nav.credit_packs',
    'app.wallet.wallet_feature_disabled',
    'app.wallet.delete_rule_title', 'app.wallet.delete_rule_confirm',
    'app.wallet.delete_rules_title', 'app.wallet.delete_rules_confirm'
  );

COMMIT;
