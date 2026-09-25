-- 000607_remove_professional_profiles.down.sql
-- Best-effort recreation of the professional profile tables.
-- Data removed by the up migration cannot be restored.

BEGIN;

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

CREATE TABLE professional_profile_members (
    profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text NOT NULL DEFAULT 'member',
    invited_by  uuid REFERENCES users(id),
    joined_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, user_id)
);

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

ALTER TABLE contents ADD COLUMN IF NOT EXISTS author_profile_id uuid REFERENCES professional_profiles(id);
ALTER TABLE comments ADD COLUMN IF NOT EXISTS author_profile_id uuid REFERENCES professional_profiles(id);

-- Cannot restore profile_id values for ai_personas; keep nullable.

COMMIT;
