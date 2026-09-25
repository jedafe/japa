-- Section 32: Activity Feed / Timeline
-- Feeds table for hybrid fan-out (push for small spaces, pull for large)

CREATE TABLE feeds (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    space_id     uuid REFERENCES spaces(id) ON DELETE SET NULL,
    content_type text NOT NULL,
    score        numeric NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, content_id)
);
CREATE INDEX idx_feeds_user_created ON feeds (tenant_id, user_id, created_at DESC);
CREATE INDEX idx_feeds_user_score ON feeds (tenant_id, user_id, score DESC);
CREATE INDEX idx_feeds_space ON feeds (tenant_id, space_id, created_at DESC) WHERE space_id IS NOT NULL;
