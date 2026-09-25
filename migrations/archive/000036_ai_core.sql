-- Section 20: AI Core — HITL review table
-- Stores human-in-the-loop review records for AI-generated output.
-- Every high-stakes AI output must pass HITL before being published.

CREATE TABLE IF NOT EXISTS ai_hitl_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    content_id uuid NOT NULL,
    content_type text NOT NULL,
    ai_output jsonb NOT NULL,
    human_feedback jsonb,
    status text NOT NULL DEFAULT 'pending',
    reviewer_id uuid REFERENCES users(id),
    reason text,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_hitl_reviews_status ON ai_hitl_reviews(tenant_id, status);
CREATE INDEX idx_ai_hitl_reviews_tenant ON ai_hitl_reviews(tenant_id, created_at DESC);
