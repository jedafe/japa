-- Section 33: Professional Profiles
-- Structured public-facing entities (agencies, consultancies, practices, brands)
-- that a user owns and can post as. Users switch authorship context when
-- creating content.

-- =====================
-- Profile Type Definitions (admin-configurable)
-- =====================
CREATE TABLE professional_profile_types (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    icon            text,
    field_group_id  uuid REFERENCES custom_field_groups(id),
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_professional_profile_types_tenant ON professional_profile_types(tenant_id);

-- =====================
-- Professional Profiles
-- =====================
CREATE TABLE professional_profiles (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    owner_user_id       uuid NOT NULL REFERENCES users(id),
    type_id             uuid NOT NULL REFERENCES professional_profile_types(id),
    name                text NOT NULL,
    slug                text NOT NULL,
    tagline             text,
    description         text,
    logo_media_id       uuid REFERENCES media(id),
    header_media_id     uuid REFERENCES media(id),
    website_url         text,
    verified            boolean NOT NULL DEFAULT false,
    verification_type   text,
    follower_count      int NOT NULL DEFAULT 0,
    status              text NOT NULL DEFAULT 'active',
    field_values        jsonb NOT NULL DEFAULT '{}',
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(slug)
);
CREATE INDEX idx_professional_profiles_owner ON professional_profiles(owner_user_id);
CREATE INDEX idx_professional_profiles_type ON professional_profiles(type_id);
CREATE INDEX idx_professional_profiles_tenant ON professional_profiles(tenant_id);
CREATE INDEX idx_professional_profiles_slug ON professional_profiles(slug);

-- =====================
-- Professional Profile Members
-- =====================
CREATE TABLE professional_profile_members (
    profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text NOT NULL DEFAULT 'member',
    invited_by  uuid REFERENCES users(id),
    joined_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, user_id)
);
CREATE INDEX idx_professional_profile_members_user ON professional_profile_members(user_id);

-- =====================
-- Professional Profile Invitations
-- =====================
CREATE TABLE professional_profile_invitations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    email       text NOT NULL,
    role        text NOT NULL DEFAULT 'member',
    invited_by  uuid REFERENCES users(id),
    token       text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_professional_profile_invitations_token ON professional_profile_invitations(token);
CREATE INDEX idx_professional_profile_invitations_profile ON professional_profile_invitations(profile_id);

-- =====================
-- Add author_profile_id to contents table
-- =====================
ALTER TABLE contents ADD COLUMN IF NOT EXISTS author_profile_id uuid REFERENCES professional_profiles(id);
