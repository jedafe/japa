-- 001175_federation_m3_discovery.up.sql
-- §126 M3 — discovery & reach: federated_actors generalizes to
-- non-member actors (kind person|application|group, nullable user_id,
-- space_id for space Group actors), the federation_relays outbound
-- subscription ledger, and the federation.authorized_fetch setting.
--
-- Ref: .jeos/sections/126-activitypub-federation.md (Discovery & Reach)

BEGIN;

ALTER TABLE federated_actors
    ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE federated_actors
    ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'person';
ALTER TABLE federated_actors
    ADD COLUMN IF NOT EXISTS space_id uuid REFERENCES spaces(id) ON DELETE CASCADE;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'federated_actors_kind_check') THEN
        ALTER TABLE federated_actors
            ADD CONSTRAINT federated_actors_kind_check
            CHECK (kind IN ('person','application','group'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'federated_actors_kind_user_match') THEN
        ALTER TABLE federated_actors
            ADD CONSTRAINT federated_actors_kind_user_match
            CHECK ((kind = 'person') = (user_id IS NOT NULL));
    END IF;
END $$;

-- singleton instance actor (one 'application' row ever)
CREATE UNIQUE INDEX IF NOT EXISTS federated_actors_application_uidx
    ON federated_actors (kind) WHERE kind = 'application';
-- one Group actor per space
CREATE UNIQUE INDEX IF NOT EXISTS federated_actors_space_uidx
    ON federated_actors (space_id) WHERE space_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_federated_actors_kind ON federated_actors (kind);

CREATE TABLE IF NOT EXISTS federation_relays (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_url    text NOT NULL UNIQUE,
    inbox_url    text NOT NULL,
    domain       text NOT NULL,
    status       text NOT NULL DEFAULT 'pending',
    follow_ap_id text,
    last_error   text,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federation_relays_status_check
        CHECK (status IN ('pending','accepted','rejected','failed','disabled'))
);
CREATE INDEX IF NOT EXISTS idx_federation_relays_follow_ap_id
    ON federation_relays (follow_ap_id) WHERE follow_ap_id IS NOT NULL;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.authorized_fetch', to_jsonb(false), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
