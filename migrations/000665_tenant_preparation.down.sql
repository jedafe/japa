-- Phase 1.0 down: restore the original tenant-aware constraints and remove defaults.

DROP INDEX IF EXISTS idx_system_settings_key;

ALTER TABLE feature_flags
DROP CONSTRAINT feature_flags_pkey,
ADD PRIMARY KEY (key, tenant_id);

DO $$
DECLARE
    rec record;
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
            'ALTER TABLE %I.%I ALTER COLUMN tenant_id DROP DEFAULT',
            rec.table_schema, rec.table_name
        );
    END LOOP;
END $$;
