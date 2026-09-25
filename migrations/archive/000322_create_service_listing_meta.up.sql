-- 000322_create_service_listing_meta.up.sql
-- Service Directory (§98, ADR-106) — service_listing_meta table.
-- Service listings are content_type = 'service_listing' in the contents table.
-- Ref: japaconnect-engineering/sections/98-service-directory.md
--      japaconnect-engineering/adr/ADR-106-service-directory.md

BEGIN;

CREATE TABLE IF NOT EXISTS service_listing_meta (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id            uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    org_id                uuid REFERENCES organizations(id) ON DELETE SET NULL,
    service_category      text NOT NULL, -- immigration_legal|relocation|finance|education|health|community|other (§98)
    destination_space_id  uuid REFERENCES spaces(id) ON DELETE SET NULL,
    city_space_id         uuid REFERENCES spaces(id) ON DELETE SET NULL,
    origin_communities    text, -- free-text community tags (altered to text[] by 000324)
    is_remote             boolean NOT NULL DEFAULT false,
    website_url           text,
    booking_url           text,
    price_range           text, -- e.g. "$$","$50-$200"
    is_verified           boolean NOT NULL DEFAULT false,
    verification_type     text, -- self|community|admin|professional_body (§98)
    claim_status          text NOT NULL DEFAULT 'unclaimed', -- unclaimed|pending|approved|rejected (§98)
    claimed_by_org_id     uuid REFERENCES organizations(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_listing_meta_tenant_dest_cat
    ON service_listing_meta(tenant_id, destination_space_id, service_category);
CREATE INDEX IF NOT EXISTS idx_service_listing_meta_tenant_verified
    ON service_listing_meta(tenant_id, is_verified)
    WHERE is_verified = true;

COMMIT;
