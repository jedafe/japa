-- 000513 — Update the external.search tool catalog entry to mention Perplexity.
--
-- Perplexity is now a supported external.search provider alongside Tavily
-- and Brave. This migration updates the seeded tool description so the
-- agent catalog reflects the new option.
--
-- Ref: internal/ai/tools_knowledge.go

UPDATE ai_tools
SET description = 'Search the web via Tavily, Brave, or Perplexity (configurable via agent.external_search_provider). Results are cached for 30 minutes. Requires agent.external_search permission.',
    config = jsonb_set(
        config,
        '{description}',
        '"Search the web when internal knowledge is insufficient. Provider is configurable via agent.external_search_provider (tavily, brave, or perplexity)."'
    ),
    updated_at = now()
WHERE name = 'external.search';
