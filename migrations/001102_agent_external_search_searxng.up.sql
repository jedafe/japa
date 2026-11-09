-- 001102_agent_external_search_searxng.up.sql
--
-- SearXNG as a fourth external.search provider (§103): a self-hosted
-- metasearch instance, so external search — and the external.fetch
-- follow-up reads — work with no paid search API key at all. The provider
-- needs only a base URL (agent.external_search_searxng_base), which may
-- point at a private LAN/Docker address; the API-key check is
-- provider-conditional in SearchWeb.
--
-- Ref: .jeos/sections/103-ai-agent-executor.md

BEGIN;

-- Instance base URL (e.g. http://localhost:8888 or http://searxng:8080).
-- The instance must have JSON output enabled in settings.yml
-- (search: formats: [html, json]) or the response decode fails.
INSERT INTO system_settings (key, value, is_public) VALUES
('agent.external_search_searxng_base', '""', false)
ON CONFLICT (key) DO NOTHING;

-- Refresh the catalog copy so admins discover the provider from the tool
-- description (mirrors 000513's description-refresh pattern).
UPDATE ai_tools
SET description = 'Search the web via Tavily, Brave, Perplexity, or a self-hosted SearXNG instance (configurable via agent.external_search_provider). SearXNG needs no API key — only agent.external_search_searxng_base. Results are cached for 30 minutes. Requires agent.external_search permission.',
    config = jsonb_set(config, '{description}', '"Search the web when internal knowledge is insufficient. Provider is configurable via agent.external_search_provider (tavily, brave, perplexity, or searxng). searxng is self-hosted and needs no API key — only a base URL."'::jsonb),
    updated_at = now()
WHERE name = 'external.search';

COMMIT;
