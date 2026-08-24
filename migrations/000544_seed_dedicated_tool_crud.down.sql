BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name ~ '^(scholarship|visa_route|housing_guide|attestation_guide|arrival_checklist|credential_pathway|language_test_requirement|tax_obligation_guide|community_organisation|wiki_page|calendar_entry|scam_report|planning_group|visa_timeline|processing_time|digest_edition|mutual_aid|embassy|appointment_alert|credential_profession|cv_formatter|language_test|mentorship_connection|remittance_rate|remittance_provider|readiness_assessment|tax_treaty_pair|travel_provider)\.(create|update|delete|assign_geo)$';

COMMIT;
