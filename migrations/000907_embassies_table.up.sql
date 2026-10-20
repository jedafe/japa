-- 000907_embassies_table.up.sql
-- JCE ↔ wiki mapping review fix: the embassy wiki type + resolver were
-- bound to monitored_embassies — a polling-config table (booking_url,
-- poll_interval, last_polled_at) that captures only the host side of the
-- relationship. A real embassies table gives embassy pages/entities the
-- dual-country model the spec flags (host = where the embassy is,
-- origin = which country it represents) and frees the wiki binding from
-- the polling concern. monitored_embassies stays as the polling config.

BEGIN;

CREATE TABLE embassies (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             text NOT NULL UNIQUE,
    name             text NOT NULL,
    host_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    origin_space_id  uuid REFERENCES spaces(id) ON DELETE SET NULL,
    address          text,
    phone            text,
    email            text,
    website          text,
    booking_url      text,
    is_active        boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX embassies_host ON embassies (host_space_id) WHERE is_active;
CREATE INDEX embassies_origin ON embassies (origin_space_id) WHERE is_active;

-- Seed from the monitored set: host = destination_space_id, origin unknown
-- (the polling table never recorded it — fill in later as data improves).
INSERT INTO embassies (slug, name, host_space_id, booking_url, is_active)
SELECT m.slug, m.name, m.destination_space_id, m.booking_url, m.is_active
FROM monitored_embassies m
WHERE m.destination_space_id IS NOT NULL
ON CONFLICT (slug) DO NOTHING;

-- Rebind the wiki entity kind + JCE resolver target to the real table.
UPDATE wiki_entity_kinds
SET source_table = 'embassies'
WHERE entity_kind = 'embassy';

COMMIT;
