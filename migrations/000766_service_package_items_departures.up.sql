-- 000766_service_package_items_departures.up.sql
-- §111 / ADR-123 — package bundle components (ordered line items) and
-- optional scheduled departures/intakes.
--
-- Ref: .jeos/sections/111-service-packages-bookings.md

BEGIN;

CREATE TABLE IF NOT EXISTS service_package_items (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    package_meta_id      uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    position             int NOT NULL DEFAULT 0,
    title                text NOT NULL,
    description          text,
    -- Optional deep-link to a standalone service listing authored by the
    -- same org. SET NULL so a deleted standalone listing degrades the item
    -- to a plain text line rather than breaking the package (§111).
    component_content_id uuid REFERENCES contents(id) ON DELETE SET NULL,
    included             boolean NOT NULL DEFAULT true,
    optional             boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_svc_pkg_items
    ON service_package_items (package_meta_id, position);

CREATE TABLE IF NOT EXISTS service_package_departures (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    package_meta_id uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    starts_on       date NOT NULL,
    ends_on         date,
    capacity        int,                            -- NULL = inherit package capacity
    booked_count    int NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open','full','cancelled')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_svc_pkg_deps
    ON service_package_departures (package_meta_id, starts_on);

COMMIT;
