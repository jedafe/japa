-- Rollback: delete the api_keys i18n keys seeded in the up migration
DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND locale IN ('en', 'fr')
  AND (
    key IN (
      'api_keys.analytics',
      'api_keys.analytics_tab',
      'api_keys.total_requests',
      'api_keys.active_keys',
      'api_keys.suspended_keys',
      'api_keys.revoked_keys',
      'api_keys.top_keys',
      'api_keys.zero_usage',
      'api_keys.recent_activity',
      'api_keys.no_activity',
      'api_keys.no_zero_usage',
      'api_keys.loading',
      'api_keys.scopes_hint',
      'api_keys.unsuspend_key',
      'api_keys.unsuspend_title',
      'api_keys.unsuspend_confirm_detail',
      'api_keys.unsuspend_text',
      'api_keys.unsuspend_error',
      'api_keys.unrevoke_key',
      'api_keys.unrevoke_title',
      'api_keys.unrevoke_confirm_detail',
      'api_keys.unrevoke_text',
      'api_keys.unrevoke_error',
      'api_keys.delete_key',
      'api_keys.delete_title',
      'api_keys.delete_confirm_detail',
      'api_keys.delete_text',
      'api_keys.delete_error',
      'api_keys.delete_selected',
      'api_keys.delete_selected_confirm'
    )
  );
