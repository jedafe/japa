-- Subscription Plan ↔ Track junction table (ADR-063 extension).
-- Links subscription_plans to learning_tracks so that specific plans can
-- include specific courses. When a plan has NO rows in this table, it
-- includes ALL tracks (backward compatible with the Pro = all model).
-- When a plan has rows, only the listed tracks are included.

CREATE TABLE subscription_plan_tracks (
    plan_id    uuid NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    track_id   uuid NOT NULL REFERENCES learning_tracks(id) ON DELETE CASCADE,
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, track_id)
);

CREATE INDEX idx_subscription_plan_tracks_plan ON subscription_plan_tracks(plan_id);
CREATE INDEX idx_subscription_plan_tracks_track ON subscription_plan_tracks(track_id);
CREATE INDEX idx_subscription_plan_tracks_tenant ON subscription_plan_tracks(tenant_id);
