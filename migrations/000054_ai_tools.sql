-- Section 22 (AI Tools): AI tool registry and usage tracking tables
-- Supports managing AI tool configurations and tracking usage statistics.

CREATE TABLE IF NOT EXISTS ai_tools (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                text NOT NULL,
    description         text NOT NULL DEFAULT '',
    category            text NOT NULL,
    config              jsonb NOT NULL DEFAULT '{}',
    is_enabled          boolean NOT NULL DEFAULT true,
    required_permission text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX idx_ai_tools_tenant ON ai_tools(tenant_id, category);

CREATE TABLE IF NOT EXISTS ai_tool_usage (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    tool_id     uuid NOT NULL REFERENCES ai_tools(id) ON DELETE CASCADE,
    user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    tokens_used int NOT NULL DEFAULT 0,
    cost_cents  int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_tool_usage_tool ON ai_tool_usage(tool_id, created_at DESC);
CREATE INDEX idx_ai_tool_usage_tenant ON ai_tool_usage(tenant_id, created_at DESC);
