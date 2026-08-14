-- Down migration for 000009_geo_first_space_system.sql
-- Drops space tables in reverse creation order and removes the seeded
-- bubble_depth system setting (system_settings table is owned by 000004).
DELETE FROM system_settings
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND key = 'spaces.bubble_depth';

DROP TABLE IF EXISTS space_rules;
DROP TABLE IF EXISTS spaces;
DROP TABLE IF EXISTS space_types;
DROP TABLE IF EXISTS space_permission_presets;
