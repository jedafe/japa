-- RBAC tables: user_groups, user_roles, permissions, and join tables.
-- Three-tier model: User Groups -> User Roles -> Permissions.
-- Three system defaults are seeded: super_admin, members, guest.

-- User Groups (e.g. "Editors", "Moderators", "Premium Members")
CREATE TABLE user_groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    slug        text NOT NULL,
    is_system   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_user_groups_tenant ON user_groups(tenant_id);

-- User Roles (intermediate tier - groups have roles, roles have permissions)
CREATE TABLE user_roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    slug        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_user_roles_tenant ON user_roles(tenant_id);

-- Permissions (the actual permission nodes, e.g. "blog.publish")
CREATE TABLE permissions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key         text NOT NULL UNIQUE,
    description text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Role-Permission mapping (many-to-many)
CREATE TABLE role_permissions (
    role_id       uuid NOT NULL REFERENCES user_roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);

-- Group-Role mapping (many-to-many)
CREATE TABLE group_roles (
    group_id    uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    role_id     uuid NOT NULL REFERENCES user_roles(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, role_id)
);

-- User-Group mapping (many-to-many - users belong to groups)
CREATE TABLE user_group_memberships (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id    uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, group_id)
);

CREATE INDEX idx_user_group_memberships_user ON user_group_memberships(tenant_id, user_id);
CREATE INDEX idx_user_group_memberships_group ON user_group_memberships(tenant_id, group_id);

-- Seed the three system default groups for the default tenant
INSERT INTO user_groups (id, tenant_id, name, slug, is_system) VALUES
    ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001', 'Super Admin', 'super_admin', true),
    ('00000000-0000-0000-0000-000000000020', '00000000-0000-0000-0000-000000000001', 'Members', 'members', true),
    ('00000000-0000-0000-0000-000000000030', '00000000-0000-0000-0000-000000000001', 'Guest', 'guest', true);
