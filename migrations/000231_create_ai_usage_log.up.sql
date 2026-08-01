-- 000231_create_ai_usage_log.up.sql
-- Section 20 (AI Core): ai_usage_log table for admin AI usage monitoring.
-- Backs GET /admin/ai/usage which returns { total_cost_usd, tokens_used, by_task_type }.
-- Every AI gateway call should write a row here for cost/usage tracking.

CREATE TABLE IF NOT EXISTS ai_usage_log (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL,
    provider        text NOT NULL,
    model           text NOT NULL,
    task_type       text NOT NULL,
    prompt_tokens   integer NOT NULL DEFAULT 0,
    completion_tokens integer NOT NULL DEFAULT 0,
    total_tokens    integer NOT NULL DEFAULT 0,
    cost_usd        numeric(12,6) NOT NULL DEFAULT 0,
    request_ref     text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_usage_log_tenant_created ON ai_usage_log(tenant_id, created_at DESC);
CREATE INDEX idx_ai_usage_log_tenant_task ON ai_usage_log(tenant_id, task_type, created_at DESC);
CREATE INDEX idx_ai_usage_log_tenant_provider ON ai_usage_log(tenant_id, provider, created_at DESC);
