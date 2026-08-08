-- 000343_create_community_locator_tables.up.sql
-- Diaspora Community Locator (§101, ADR-109) — community_organisations.
--
-- Adaptation: §101 references sites(id), geo_countries(id), geo_cities(id),
-- spaces(id) (BIGINT), organizations(id) (BIGINT). Per ADR-053, tenancy uses
-- tenants(id) (uuid), countries/cities are spaces with level discriminator,
-- spaces and organizations use uuid PKs.
--
-- Note: GiST spatial index on point(longitude, latitude) omitted because
-- PostGIS may not be available; lat/lng columns are indexed btree for
-- bounding-box queries instead.
--
-- Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md

BEGIN;

CREATE TABLE IF NOT EXISTS community_organisations (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                    text NOT NULL,
    slug                    text NOT NULL,
    org_type                text NOT NULL, -- cultural_association|church|mosque|professional_network|student_union|informal_group|other
    origin_community        text NOT NULL, -- e.g. 'Nigerian','Ghanaian','West African','African'
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    city_space_id           uuid REFERENCES spaces(id) ON DELETE SET NULL,
    address                 text,
    latitude                numeric(9,6),
    longitude               numeric(9,6),
    website_url             text,
    meeting_frequency       text,
    contact_info            text,
    japaconnect_space_id    uuid REFERENCES spaces(id) ON DELETE SET NULL,
    japaconnect_org_id      uuid REFERENCES organizations(id) ON DELETE SET NULL,
    claim_status            text NOT NULL DEFAULT 'unclaimed', -- unclaimed|pending|claimed
    is_verified             boolean NOT NULL DEFAULT false,
    is_active               boolean NOT NULL DEFAULT true,
    is_community_submitted  boolean NOT NULL DEFAULT true,
    status                  text NOT NULL DEFAULT 'pending', -- pending|published|archived
    submitted_by            uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at             timestamptz,
    is_japabot_indexed      boolean NOT NULL DEFAULT false,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz, -- Rule 11: soft-delete user content
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_dest_status
    ON community_organisations(tenant_id, destination_space_id, status);
CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_origin
    ON community_organisations(tenant_id, origin_community, status);
CREATE INDEX IF NOT EXISTS idx_community_orgs_tenant_type
    ON community_organisations(tenant_id, org_type, status);
CREATE INDEX IF NOT EXISTS idx_community_orgs_lat_lng
    ON community_organisations(latitude, longitude)
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;

COMMIT;
