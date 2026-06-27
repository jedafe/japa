-- Section 43: Bot & Crawler Management

-- =====================
-- Known Bots
-- =====================

CREATE TABLE known_bots (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name               text NOT NULL,
    user_agent_pattern text NOT NULL,
    bot_type           text NOT NULL DEFAULT 'unknown',
    -- 'search_engine'|'ai_llm'|'social'|'monitoring'|'scraper'|'unknown'
    policy             text NOT NULL DEFAULT 'allow',
    -- 'allow'|'block'|'limit'
    is_system          boolean NOT NULL DEFAULT false,
    crawl_delay        integer NOT NULL DEFAULT 0,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_known_bots_tenant ON known_bots(tenant_id);
CREATE INDEX idx_known_bots_type ON known_bots(tenant_id, bot_type);

-- Seed known bots for default tenant (idempotent)
INSERT INTO known_bots (tenant_id, name, user_agent_pattern, bot_type, policy, is_system)
VALUES
    -- Default tenant UUID
    ('00000000-0000-0000-0000-000000000001', 'Googlebot', 'Googlebot', 'search_engine', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'Bingbot', 'bingbot', 'search_engine', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'GPTBot', 'GPTBot', 'ai_llm', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'ClaudeBot', 'ClaudeBot', 'ai_llm', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'PerplexityBot', 'PerplexityBot', 'ai_llm', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'Twitterbot', 'Twitterbot', 'social', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'LinkedInBot', 'LinkedInBot', 'social', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'AhrefsBot', 'AhrefsBot', 'monitoring', 'allow', true),
    ('00000000-0000-0000-0000-000000000001', 'SemrushBot', 'SemrushBot', 'monitoring', 'allow', true)
ON CONFLICT DO NOTHING;
