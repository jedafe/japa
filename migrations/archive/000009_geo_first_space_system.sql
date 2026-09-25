-- Geo-First Space System (Section 05).
-- Spaces are the geo-first containers for community activity: Country,
-- State, City, University, and Community spaces form a hierarchy via
-- parent_id. Each space carries five admin-configurable permission axes
-- (visibility, posting, replying, commenting, joining) initialised from a
-- permission preset at creation time. Space Types optionally link to a
-- custom_field_group (Section 06) for type-specific fields. Seven system
-- permission presets, five system space types, and the bubble_depth
-- setting are seeded.

-- Space Permission Presets (seven defaults, admin-editable).
-- Each preset defines the five permission axes applied when a space is
-- created with this preset. Spaces store their actual axis values
-- directly; presets are convenience defaults.
CREATE TABLE space_permission_presets (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,  -- 'open_community', 'geo_space', etc.
    visibility      text NOT NULL,  -- 'public' | 'members' | 'private'
    posting_perm    text NOT NULL,  -- 'open' | 'members' | 'verified' | 'moderators' | 'admins'
    replying_perm   text NOT NULL,  -- same options as posting_perm
    commenting_perm text NOT NULL,  -- same options as posting_perm
    joining_perm    text NOT NULL,  -- 'open' | 'request' | 'invite'
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_space_permission_presets_tenant ON space_permission_presets(tenant_id);

-- Space Types (Country, City, University, Community, etc.)
-- Each type can optionally link to a custom_field_group (Section 06)
-- for type-specific fields.
CREATE TABLE space_types (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,  -- 'Country', 'City', 'University', 'Community'
    slug            text NOT NULL,  -- 'country', 'city', 'university', 'community'
    description     text,
    icon            text,  -- icon name or URL
    field_group_id  uuid REFERENCES custom_field_groups(id) ON DELETE SET NULL,
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_space_types_tenant ON space_types(tenant_id);

-- Spaces (the actual spaces with hierarchy, geo classification, and
-- five permission axes). parent_id creates the country -> state -> city
-- hierarchy. logo_media_id and header_image_id are plain uuid WITHOUT FK
-- constraints because the media table (Section 34) does not exist yet; FK
-- constraints will be added in a later migration when the media system is
-- built.
CREATE TABLE spaces (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_id         uuid REFERENCES spaces(id) ON DELETE CASCADE,
    space_type_id     uuid NOT NULL REFERENCES space_types(id),
    name              text NOT NULL,
    slug              text NOT NULL,
    description       text,
    wiki_content      jsonb,  -- TipTap JSON for the rich wiki tab
    logo_media_id     uuid,  -- FK to media(id) added in Phase 3 (Section 34)
    header_image_id   uuid,  -- FK to media(id) added in Phase 3 (Section 34)
    geo_code          text,  -- ISO code for country/state
    level             text NOT NULL,  -- 'country' | 'state' | 'city' | 'custom'
    member_count      int NOT NULL DEFAULT 0,
    post_count        int NOT NULL DEFAULT 0,
    follower_count    int NOT NULL DEFAULT 0,
    visibility        text NOT NULL DEFAULT 'public',
    posting_perm      text NOT NULL DEFAULT 'members',
    replying_perm     text NOT NULL DEFAULT 'members',
    commenting_perm   text NOT NULL DEFAULT 'members',
    joining_perm      text NOT NULL DEFAULT 'open',
    status            text NOT NULL DEFAULT 'active',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_spaces_tenant ON spaces(tenant_id);
CREATE INDEX idx_spaces_parent ON spaces(parent_id);
CREATE INDEX idx_spaces_type ON spaces(space_type_id);
CREATE INDEX idx_spaces_geo ON spaces(geo_code) WHERE geo_code IS NOT NULL;
CREATE INDEX idx_spaces_level ON spaces(tenant_id, level);

-- Space Rules (per-space rules shown in the space's Rules tab).
CREATE TABLE space_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    title       text NOT NULL,
    body        text NOT NULL,
    position    int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_space_rules_space ON space_rules(space_id);

-- Seed the seven system default permission presets for the default tenant.
-- UUIDs continue the zeros-pattern: 010/020/030 are RBAC groups, 040-049
-- are custom fields (Section 06), so permission presets start at 050.
INSERT INTO space_permission_presets (id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system) VALUES
    ('00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001', 'open_community', 'public', 'members', 'members', 'members', 'open', true),
    ('00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000001', 'geo_space', 'public', 'members', 'members', 'members', 'open', true),
    ('00000000-0000-0000-0000-000000000052', '00000000-0000-0000-0000-000000000001', 'curated', 'public', 'moderators', 'members', 'members', 'request', true),
    ('00000000-0000-0000-0000-000000000053', '00000000-0000-0000-0000-000000000001', 'private', 'members', 'members', 'members', 'members', 'invite', true),
    ('00000000-0000-0000-0000-000000000054', '00000000-0000-0000-0000-000000000001', 'read_only', 'public', 'moderators', 'moderators', 'moderators', 'open', true),
    ('00000000-0000-0000-0000-000000000055', '00000000-0000-0000-0000-000000000001', 'verified_only', 'public', 'verified', 'members', 'members', 'open', true),
    ('00000000-0000-0000-0000-000000000056', '00000000-0000-0000-0000-000000000001', 'admin_only', 'members', 'admins', 'admins', 'admins', 'invite', true);

-- Seed the five system default space types for the default tenant.
-- Country, State, City, and Community link to the space_fields group (043);
-- University links to the university_fields group (044) from Section 06.
INSERT INTO space_types (id, tenant_id, name, slug, description, icon, field_group_id, is_system) VALUES
    ('00000000-0000-0000-0000-000000000060', '00000000-0000-0000-0000-000000000001', 'Country', 'country', 'Country-level geo space', 'globe', '00000000-0000-0000-0000-000000000043', true),
    ('00000000-0000-0000-0000-000000000061', '00000000-0000-0000-0000-000000000001', 'State', 'state', 'State or province level geo space', 'map', '00000000-0000-0000-0000-000000000043', true),
    ('00000000-0000-0000-0000-000000000062', '00000000-0000-0000-0000-000000000001', 'City', 'city', 'City-level geo space', 'map-pin', '00000000-0000-0000-0000-000000000043', true),
    ('00000000-0000-0000-0000-000000000063', '00000000-0000-0000-0000-000000000001', 'Community', 'community', 'General community space', 'users', '00000000-0000-0000-0000-000000000043', true),
    ('00000000-0000-0000-0000-000000000064', '00000000-0000-0000-0000-000000000001', 'University', 'university', 'University space with academic fields', 'graduation-cap', '00000000-0000-0000-0000-000000000044', true);

-- Seed the post bubbling depth cap as a system setting.
-- Controls how deep reply threads nest before collapsing into a "view more" bubble.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'spaces.bubble_depth', '2'::jsonb);
