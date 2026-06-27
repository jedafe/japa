-- 000031_onboarding.sql
-- Onboarding state tracking per user.

CREATE TABLE onboarding_state (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    current_step    int NOT NULL DEFAULT 1,
    completed       boolean NOT NULL DEFAULT false,
    step_data       jsonb NOT NULL DEFAULT '{}',
    tour_dismissed  boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX idx_onboarding_state_tenant ON onboarding_state(tenant_id);
