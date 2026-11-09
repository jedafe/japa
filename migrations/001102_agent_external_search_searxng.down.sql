-- 001102_agent_external_search_searxng.down.sql
--
-- Removes the SearXNG base-URL setting and reverts the external.search
-- catalog copy to the pre-searxng text (000513).

BEGIN;

DELETE FROM system_settings WHERE key = 'agent.external_search_searxng_base';

UPDATE ai_tools
SET description = 'Search the web via Tavily, Brave, or Perplexity (configurable via agent.external_search_provider). Results are cached for 30 minutes. Requires agent.external_search permission.',
    config = jsonb_set(config, '{description}', '"Search the web when internal knowledge is insufficient. Provider is configurable via agent.external_search_provider (tavily, brave, or perplexity)."'::jsonb),
    updated_at = now()
WHERE name = 'external.search';

COMMIT;
