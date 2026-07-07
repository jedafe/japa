-- Phase 28: Trending & Discovery (Section 53)
-- Pre-computed trending snapshots refreshed hourly by Asynq task.

CREATE TABLE IF NOT EXISTS trending_snapshots (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id),
    entity_type   text NOT NULL, -- 'content', 'tag', 'space', 'user'
    entity_id     uuid NOT NULL,
    score         float NOT NULL DEFAULT 0,
    "window"      text NOT NULL DEFAULT '24h', -- '24h', '7d', '30d'
    geo_code      text, -- country code, NULL for global
    computed_at   timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, entity_type, entity_id, "window", geo_code)
);

CREATE INDEX idx_trending_snapshots_tenant_entity ON trending_snapshots (tenant_id, entity_type, entity_id);
CREATE INDEX idx_trending_snapshots_trending ON trending_snapshots (tenant_id, entity_type, "window", geo_code, score DESC);
CREATE INDEX idx_trending_snapshots_expires ON trending_snapshots (expires_at);
