-- Audit every scoped table to confirm it contains only the default tenant.
-- Returns one row per (table, tenant_id) with the row count.
CREATE TEMP TABLE IF NOT EXISTS audit_results (
    table_name text,
    tenant_id uuid,
    rows bigint
);

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
            'INSERT INTO audit_results SELECT %L, tenant_id, count(*) FROM %I.%I GROUP BY tenant_id',
            rec.table_name, rec.table_schema, rec.table_name
        );
    END LOOP;
END $$;

SELECT table_name, tenant_id, rows
FROM audit_results
ORDER BY table_name, tenant_id;

DROP TABLE audit_results;
