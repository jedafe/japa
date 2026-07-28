-- 000182_geo_archetype_config.down.sql
-- Rollback: remove the destination_country custom field and the two
-- geo_personalization market-list settings (ADR-064 / Phase 38.03).

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('geo_personalization.origin_markets', 'geo_personalization.destination_markets');

DELETE FROM custom_fields
WHERE id = '00000000-0000-0000-0000-000000000050';
