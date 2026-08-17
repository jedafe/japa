-- 000511_community_locator_gist_point_index.up.sql
-- BUG-007: Add a GiST spatial index for proximity queries on
-- community_organisations. Uses PostgreSQL's CORE point type (no
-- PostGIS extension required) — the point geometric type has built-in
-- GiST support via gist_point_ops and the <-> distance operator.
--
-- This replaces the btree bounding-box approximation (000343) with a
-- true spatial index that supports both radius filtering and
-- nearest-first ordering via the <-> operator.
--
-- Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
--      japaconnect-engineering/adr/ADR-109-community-locator.md
--      PostgreSQL docs: "Geometric Types" / "GiST Indexes" (point type)

BEGIN;

-- Expression GiST index on point(longitude, latitude). The point type
-- is a core PostgreSQL geometric type; gist_point_ops is included in
-- the default opclass set (no CREATE EXTENSION needed). The <->
-- operator returns Euclidean distance between two points, which is
-- sufficient for city-scale proximity queries (the existing bounding-
-- box code already used the same degree-based approximation).
CREATE INDEX IF NOT EXISTS idx_community_orgs_gist_point
    ON community_organisations
    USING GIST (point(longitude, latitude))
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;

-- The btree index from 000343 is now redundant for proximity queries
-- but is retained as a fallback for non-proximity latitude/longitude
-- range scans. No action needed.

COMMIT;
