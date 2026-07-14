-- Remove missing Control App navigation keys
DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
    AND locale = 'en'
    AND "key" IN ('nav.dlq', 'nav.feature_flags', 'nav.tenants', 'nav.ai_budget', 'nav.cron');
