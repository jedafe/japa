-- Phase 1.0: make tenant_id optional and prepare system_settings / feature_flags for single-tenant operation.
-- This migration does NOT drop tenant_id columns; it only sets defaults and rewrites the
-- constraints needed by the settings and feature flag subsystems.

DO $$
DECLARE
    rec record;
    default_tenant uuid := '00000000-0000-0000-0000-000000000001'::uuid;
BEGIN
    FOR rec IN
        SELECT c.table_schema, c.table_name
        FROM information_schema.columns c
        JOIN information_schema.tables t
          ON c.table_name = t.table_name AND c.table_schema = t.table_schema
        WHERE c.column_name = 'tenant_id'
          AND c.table_schema = 'public'
          AND t.table_type = 'BASE TABLE'
        ORDER BY c.table_name
    LOOP
        EXECUTE format(
            'ALTER TABLE %I.%I ALTER COLUMN tenant_id SET DEFAULT %L',
            rec.table_schema, rec.table_name, default_tenant
        );
    END LOOP;
END $$;

-- system_settings: add a unique index on key alone so ON CONFLICT (key) and WHERE key = $1 work.
CREATE UNIQUE INDEX idx_system_settings_key ON system_settings(key);

-- feature_flags: change the primary key from (key, tenant_id) to (key) so lookups are by key only.
ALTER TABLE feature_flags
DROP CONSTRAINT feature_flags_pkey,
ADD PRIMARY KEY (key);
