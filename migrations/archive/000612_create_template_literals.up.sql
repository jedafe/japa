-- 000572_create_template_literals.up.sql
-- Creates tenant-scoped template literals for server-side text token resolution.

CREATE TABLE template_literals (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key           text NOT NULL,
    kind          text NOT NULL DEFAULT 'expression',
    value         text NOT NULL DEFAULT '',
    fallback      text,
    label         text,
    description   text,
    category      text,
    is_system     boolean NOT NULL DEFAULT false,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE UNIQUE INDEX idx_template_literals_tenant_key ON template_literals(tenant_id, key) WHERE deleted_at IS NULL;
CREATE INDEX idx_template_literals_tenant_active ON template_literals(tenant_id, is_active) WHERE deleted_at IS NULL;

-- Seed system tokens for the default tenant.
INSERT INTO template_literals (tenant_id, key, kind, value, fallback, label, description, category, is_system)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'Current_Year',  'expression', 'year(Now)',                  '',            'Current Year',      'The current calendar year.',                                'time',    true),
    ('00000000-0000-0000-0000-000000000001', 'Current_Date',  'expression', 'format(Now, "YYYY-MM-DD")',  '',            'Current Date',      'The current date.',                                         'time',    true),
    ('00000000-0000-0000-0000-000000000001', 'Current_Time',  'expression', 'format(Now, "HH:mm")',       '',            'Current Time',      'The current time.',                                         'time',    true),
    ('00000000-0000-0000-0000-000000000001', 'Source_Country','expression', 'Source_Country',             'your country','Source Country',    'The visitor''s detected source country.',                  'geo',     true),
    ('00000000-0000-0000-0000-000000000001', 'Source_Code',   'expression', 'Source_Code',                'XX',          'Source Code',       'The visitor''s detected source country code.',             'geo',     true),
    ('00000000-0000-0000-0000-000000000001', 'Destination_Country','expression','Destination_Country',     'your destination','Destination Country','The visitor''s selected destination country.',           'geo',     true),
    ('00000000-0000-0000-0000-000000000001', 'Destination_Code',   'expression','Destination_Code',        'XX',          'Destination Code',  'The visitor''s selected destination country code.',        'geo',     true),
    ('00000000-0000-0000-0000-000000000001', 'Tenant_Name',   'expression', 'Tenant_Name',                '',            'Tenant Name',       'The current tenant name.',                                  'tenant',  true),
    ('00000000-0000-0000-0000-000000000001', 'Locale',        'expression', 'Locale',                     'en',          'Locale',            'The active locale.',                                        'locale',  true);
