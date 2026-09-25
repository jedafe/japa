-- 000003_create_users_table.sql
-- Create users and user_profiles tables.
-- Every scoped table carries tenant_id for multi-tenant isolation.
-- profile_type supports personal, business, and organisation accounts.
-- password_hash is nullable to allow OAuth-only users.

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id),
    email             text NOT NULL,
    email_verified_at timestamptz,
    password_hash     text,
    username          text NOT NULL,
    profile_type      text NOT NULL DEFAULT 'personal',
    status            text NOT NULL DEFAULT 'active',
    last_login_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email),
    UNIQUE(tenant_id, username)
);

CREATE TABLE user_profiles (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name        text,
    avatar_url          text,
    bio                 text,
    timezone            text,
    business_name       text,
    registration_number text,
    operating_hours     jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_user_profiles_tenant ON user_profiles(tenant_id);
CREATE INDEX idx_users_email ON users(tenant_id, email);
CREATE INDEX idx_users_username ON users(tenant_id, username);
