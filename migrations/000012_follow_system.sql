-- Section 31: Follow System
-- Polymorphic follows for users, spaces, tags, organizations, professional_profiles.
-- Denormalized follower_count maintained in the service layer (not via triggers)
-- because not all entity tables exist yet.

-- Tags table (created here since it does not yet exist).
CREATE TABLE tags (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text,
    color           text,
    follower_count  int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_tags_tenant ON tags(tenant_id);

-- Follows table (polymorphic).
CREATE TABLE follows (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    follower_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type     text NOT NULL CHECK (entity_type IN ('user', 'space', 'tag', 'organization', 'professional_profile')),
    entity_id       uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(follower_id, entity_type, entity_id)
);
CREATE INDEX idx_follows_entity ON follows (tenant_id, entity_type, entity_id);
CREATE INDEX idx_follows_follower ON follows (tenant_id, follower_id, entity_type);

-- Follower count columns on entity tables.
ALTER TABLE users ADD COLUMN IF NOT EXISTS follower_count int NOT NULL DEFAULT 0;
-- spaces already has follower_count from Section 05.

-- Organizations table does not exist yet; skip silently.
-- professional_profiles table does not exist yet; skip silently.
