-- Phase 19: Organizations, members, invitations, and ownership claims.
-- Migration 000105: creates organizations, organization_members,
-- organization_invitations, and ownership_claims tables.

-- =====================
-- Organizations
-- =====================
CREATE TABLE IF NOT EXISTS organizations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    owner_user_id   uuid NOT NULL REFERENCES users(id),
    name            text NOT NULL,
    slug            text NOT NULL,
    tagline         text,
    description     text,
    logo_media_id   uuid REFERENCES media(id) ON DELETE SET NULL,
    header_media_id uuid REFERENCES media(id) ON DELETE SET NULL,
    website_url     text,
    country_code    text,
    geo_space_id    uuid REFERENCES spaces(id) ON DELETE SET NULL,
    org_type        text NOT NULL DEFAULT 'business'
                      CHECK (org_type IN ('business','ngo','government','embassy','university','media','community')),
    verified        boolean NOT NULL DEFAULT false,
    verification_type text,
    follower_count  int NOT NULL DEFAULT 0,
    member_count    int NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','suspended','pending_verification')),
    field_values    jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_orgs_geo ON organizations (geo_space_id) WHERE geo_space_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orgs_tenant ON organizations (tenant_id, status, created_at DESC);

-- =====================
-- Organization Members
-- =====================
CREATE TABLE IF NOT EXISTS organization_members (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            text NOT NULL DEFAULT 'member'
                      CHECK (role IN ('owner','admin','editor','member')),
    invited_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    joined_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

-- =====================
-- Organization Invitations
-- =====================
CREATE TABLE IF NOT EXISTS organization_invitations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           text NOT NULL,
    role            text NOT NULL DEFAULT 'member'
                      CHECK (role IN ('admin','editor','member')),
    invited_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    token_hash      text NOT NULL UNIQUE,
    expires_at      timestamptz NOT NULL DEFAULT now() + interval '7 days',
    accepted_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- =====================
-- Ownership Claims
-- =====================
CREATE TABLE IF NOT EXISTS ownership_claims (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    claimant_user_id uuid NOT NULL REFERENCES users(id),
    organization_id uuid NOT NULL REFERENCES organizations(id),
    evidence_url    text,
    notes           text,
    status          text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','approved','rejected','withdrawn')),
    reviewed_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    resolution_note text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    reviewed_at     timestamptz
);

CREATE INDEX IF NOT EXISTS idx_org_claims_tenant ON ownership_claims (tenant_id, status, created_at DESC);
