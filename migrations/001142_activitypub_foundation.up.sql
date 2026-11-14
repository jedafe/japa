-- 001142_activitypub_foundation.up.sql
-- ActivityPub Federation (§126, ADR-138) — identity + delivery schema:
-- federated_actors (per-member opt-in Person actors w/ encrypted RSA keys),
-- remote_actors (resolved remote actor cache), federation_follows
-- (remote→local follows), federation_deliveries (outbound ledger),
-- federation_inbound (inbound dedup/audit), federation_instance_blocks
-- (admin domain blocklist). RBAC nodes seeded complete (BE-005).
--
-- Ref: japaconnect-engineering/sections/126-activitypub-federation.md

BEGIN;

CREATE TABLE IF NOT EXISTS federated_actors (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    actor_url                   text NOT NULL UNIQUE,
    preferred_username          text NOT NULL,
    public_key_pem              text NOT NULL,
    private_key_enc             text NOT NULL,
    public_key_id               text NOT NULL,
    status                      text NOT NULL DEFAULT 'active',
    inbox_url                   text NOT NULL,
    outbox_url                  text NOT NULL,
    followers_url               text NOT NULL,
    following_url               text NOT NULL,
    manually_approves_followers boolean NOT NULL DEFAULT false,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federated_actors_status_check CHECK (status IN ('active','disabled','tombstone'))
);
-- acct localpart lookups compare under lower() — functional index serves them
CREATE INDEX IF NOT EXISTS idx_federated_actors_preferred_username ON federated_actors (lower(preferred_username));

CREATE TABLE IF NOT EXISTS remote_actors (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_url          text NOT NULL UNIQUE,
    acct               text,
    domain             text NOT NULL,
    inbox_url          text,
    shared_inbox_url   text,
    outbox_url         text,
    public_key_pem     text,
    public_key_id      text,
    preferred_username text,
    display_name       text,
    avatar_url         text,
    status             text NOT NULL DEFAULT 'active',
    fetched_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT remote_actors_status_check CHECK (status IN ('active','deleted','tombstone','blocked'))
);
CREATE INDEX IF NOT EXISTS idx_remote_actors_domain ON remote_actors (domain);

CREATE TABLE IF NOT EXISTS federation_follows (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    local_actor_id  uuid NOT NULL REFERENCES federated_actors(id) ON DELETE CASCADE,
    remote_actor_id uuid NOT NULL REFERENCES remote_actors(id) ON DELETE CASCADE,
    ap_activity_id  text,
    status          text NOT NULL DEFAULT 'pending',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (local_actor_id, remote_actor_id),
    CONSTRAINT federation_follows_status_check CHECK (status IN ('pending','accepted','rejected','undone'))
);
CREATE INDEX IF NOT EXISTS idx_federation_follows_remote ON federation_follows (remote_actor_id);

CREATE TABLE IF NOT EXISTS federation_deliveries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_ap_id  text NOT NULL,
    activity_type   text NOT NULL,
    activity_json   jsonb NOT NULL,
    local_actor_id  uuid REFERENCES federated_actors(id) ON DELETE SET NULL,
    inbox_url       text NOT NULL,
    remote_actor_id uuid REFERENCES remote_actors(id) ON DELETE SET NULL,
    status          text NOT NULL DEFAULT 'pending',
    attempts        int NOT NULL DEFAULT 0,
    next_retry_at   timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    delivered_at    timestamptz,
    CONSTRAINT federation_deliveries_status_check CHECK (status IN ('pending','delivered','failed','dead'))
);
-- worker claim index: poll due deliveries
CREATE INDEX IF NOT EXISTS idx_federation_deliveries_due ON federation_deliveries (status, next_retry_at)
    WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_federation_deliveries_activity ON federation_deliveries (activity_ap_id);

CREATE TABLE IF NOT EXISTS federation_inbound (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_ap_id  text,
    activity_type   text NOT NULL,
    actor_url       text,
    key_id          text,
    signature_valid boolean NOT NULL DEFAULT false,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'received',
    error           text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federation_inbound_status_check CHECK (status IN ('received','processed','ignored','rejected'))
);
-- dedup: remote activities may be replayed; anonymous (id-less) activities always insert
CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_inbound_ap_id ON federation_inbound (activity_ap_id)
    WHERE activity_ap_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS federation_instance_blocks (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    domain     text NOT NULL UNIQUE,
    reason     text,
    blocked_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- RBAC nodes (BE-005 — seed completely):
--   federation.use              member opt-in routes (/me/federation)
--   federation.actors.view      admin actor list + federation stats
--   federation.instances.manage admin domain block CRUD
INSERT INTO permissions (key, description) VALUES
    ('federation.use',              'Opt into ActivityPub federation (mint and manage own fediverse actor)'),
    ('federation.actors.view',      'View federated actors and federation stats (admin)'),
    ('federation.instances.manage', 'Create and remove federation instance (domain) blocks')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE (
    (r.slug IN ('member','pro_member','business_member','super_admin') AND p.key = 'federation.use')
    OR
    (r.slug = 'super_admin' AND p.key IN ('federation.actors.view','federation.instances.manage'))
)
ON CONFLICT DO NOTHING;

COMMIT;
