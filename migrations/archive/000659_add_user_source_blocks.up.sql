BEGIN;

CREATE TABLE user_source_blocks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    blocker_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id   uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, blocker_id, source_id)
);

CREATE INDEX idx_user_source_blocks_blocker ON user_source_blocks(tenant_id, blocker_id);
CREATE INDEX idx_user_source_blocks_source ON user_source_blocks(tenant_id, source_id);

COMMIT;
