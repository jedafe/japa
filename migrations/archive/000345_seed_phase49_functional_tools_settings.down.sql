-- 000345_seed_phase49_functional_tools_settings.down.sql
BEGIN;
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'visa_routes.enabled','visa_routes.subdomain','visa_routes.custom_domain',
    'appointment_alert.enabled','appointment_alert.min_poll_seconds',
    'arrival_checklist.enabled','arrival_checklist.subdomain','arrival_checklist.custom_domain',
    'credential_checker.enabled','credential_checker.subdomain','credential_checker.custom_domain',
    'cv_formatter.enabled','cv_formatter.subdomain','cv_formatter.custom_domain','cv_formatter.quota_standard',
    'housing_guide.enabled','housing_guide.subdomain','housing_guide.custom_domain',
    'attestation_guide.enabled','attestation_guide.subdomain','attestation_guide.custom_domain',
    'language_test_hub.enabled','language_test_hub.subdomain','language_test_hub.custom_domain',
    'tax_calculator.enabled','tax_calculator.subdomain','tax_calculator.custom_domain',
    'community_locator.enabled','community_locator.subdomain','community_locator.custom_domain'
  );
COMMIT;
