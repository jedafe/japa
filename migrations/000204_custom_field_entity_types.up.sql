-- Custom Field Entity Types (Section 06).
-- DB-backed registry of entity types that custom field groups can be
-- assigned to. Replaces the hardcoded list in ListEntityTypes handler.
-- Tenant-scoped so each tenant can define its own entity types; system
-- types are seeded with is_system=true and cannot be deleted.

CREATE TABLE custom_field_entity_types (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    value       text NOT NULL,          -- machine-readable token, e.g. 'user_profile'
    label       text NOT NULL,          -- human-readable label, e.g. 'User Profile'
    is_system   boolean NOT NULL DEFAULT false,  -- system types cannot be deleted
    sort_order  int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, value)
);

CREATE INDEX idx_custom_field_entity_types_tenant ON custom_field_entity_types(tenant_id);

-- Seed the system entity types that match the previous hardcoded list.
INSERT INTO custom_field_entity_types (tenant_id, value, label, is_system, sort_order) VALUES
    ('00000000-0000-0000-0000-000000000001', 'user_profile',   'User Profile',   true, 1),
    ('00000000-0000-0000-0000-000000000001', 'content',        'Content',        true, 2),
    ('00000000-0000-0000-0000-000000000001', 'space',          'Space',          true, 3),
    ('00000000-0000-0000-0000-000000000001', 'organization',   'Organization',   true, 4),
    ('00000000-0000-0000-0000-000000000001', 'tag',            'Tag',            true, 5),
    ('00000000-0000-0000-0000-000000000001', 'ticket_type',    'Ticket Type',    true, 6),
    ('00000000-0000-0000-0000-000000000001', 'form',           'Form',           true, 7)
ON CONFLICT (tenant_id, value) DO NOTHING;
