-- 000247_geo_awareness_extension.down.sql
-- Rollback: remove origin_country custom field and geo_personalization
-- settings added by ADR-064a.

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'geo_personalization.stage_tool_matrix',
    'geo_personalization.city_level_enabled',
    'geo_personalization.nominatim_url',
    'geo_personalization.nominatim_timeout_ms',
    'geo_personalization.tool_labels',
    'geo_personalization.tool_urls'
  );

DELETE FROM custom_fields
WHERE id = '00000000-0000-0000-0000-000000000051';
