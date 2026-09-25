-- 000448_intelligence_domain_catalog.down.sql
BEGIN;

DELETE FROM translations
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND "key" IN (
    'intel.category_housing',
    'intel.category_cost_of_living',
    'intel.category_travel_routes',
    'intel.category_grants',
    'intel.category_admission',
    'intel.category_banking',
    'intel.category_work_career',
    'intel.category_health_insurance',
    'intel.category_culture_lifestyle'
  );

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'intel.travel_routes.enabled',
    'intel.travel_routes.freshness_decay_policy',
    'intel.grants.enabled',
    'intel.grants.freshness_decay_policy',
    'intel.admission.enabled',
    'intel.admission.freshness_decay_policy',
    'intel.banking.enabled',
    'intel.banking.freshness_decay_policy',
    'intel.work_career.enabled',
    'intel.work_career.freshness_decay_policy',
    'intel.health_insurance.enabled',
    'intel.health_insurance.freshness_decay_policy',
    'intel.culture_lifestyle.enabled',
    'intel.culture_lifestyle.freshness_decay_policy'
  );

COMMIT;
