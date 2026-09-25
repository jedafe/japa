-- Rollback: remove the university.* and geo_personalization.enabled settings.

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
        'university.subdomain',
        'university.custom_domain',
        'university.creator_percentage_default',
        'university.platform_percentage_default',
        'university.payout_schedule',
        'university.min_payout_kobo',
        'university.pro_stream_rate_kobo',
        'geo_personalization.enabled'
  );
