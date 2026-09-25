-- Section 30: Generic Comparison System
-- Two-layer comparison: admin-defined Comparison Types with custom fields,
-- and content-linked comparison items, dimensions, and values.

-- =====================
-- Comparison Type Definitions (admin-configurable)
-- =====================
CREATE TABLE comparison_type_definitions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text,
    icon            text,
    field_group_id  uuid NOT NULL REFERENCES custom_field_groups(id),
    item_label      text NOT NULL DEFAULT 'Item',
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_comparison_type_definitions_tenant ON comparison_type_definitions(tenant_id);

-- =====================
-- Comparison Type Access (per-UserGroup permissions)
-- =====================
CREATE TABLE comparison_type_access (
    type_id         uuid NOT NULL REFERENCES comparison_type_definitions(id) ON DELETE CASCADE,
    user_group_id   uuid REFERENCES user_groups(id),
    group_key       uuid NOT NULL GENERATED ALWAYS AS (COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000')) STORED,
    can_create      boolean NOT NULL DEFAULT false,
    can_fill        boolean NOT NULL DEFAULT true,
    ai_can_fill     boolean NOT NULL DEFAULT false,
    PRIMARY KEY (type_id, group_key)
);
CREATE INDEX idx_comparison_type_access_type ON comparison_type_access(type_id);

-- =====================
-- Comparison Items (linked to a content_type='comparison' row)
-- =====================
CREATE TABLE comparison_items (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title           text NOT NULL,
    description     text,
    image_id        uuid,
    geo_space_id    uuid,
    position        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_comparison_items_content ON comparison_items(content_id);
CREATE INDEX idx_comparison_items_tenant ON comparison_items(tenant_id);

-- =====================
-- Comparison Dimensions (axes of comparison)
-- =====================
CREATE TABLE comparison_dimensions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label           text NOT NULL,
    dimension_type  text NOT NULL CHECK (dimension_type IN ('text', 'rating', 'boolean', 'number', 'currency')),
    position        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_comparison_dimensions_content ON comparison_dimensions(content_id);
CREATE INDEX idx_comparison_dimensions_tenant ON comparison_dimensions(tenant_id);

-- =====================
-- Comparison Values (dimension × item intersection)
-- =====================
CREATE TABLE comparison_values (
    dimension_id    uuid NOT NULL REFERENCES comparison_dimensions(id) ON DELETE CASCADE,
    item_id         uuid NOT NULL REFERENCES comparison_items(id) ON DELETE CASCADE,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    value           text,
    value_rating    numeric(3,2),
    value_boolean   boolean,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (dimension_id, item_id)
);
CREATE INDEX idx_comparison_values_dimension ON comparison_values(dimension_id);
CREATE INDEX idx_comparison_values_item ON comparison_values(item_id);
CREATE INDEX idx_comparison_values_tenant ON comparison_values(tenant_id);
