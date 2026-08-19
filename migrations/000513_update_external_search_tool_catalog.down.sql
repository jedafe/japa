-- 000513 — Reverses 000513_update_external_search_tool_catalog.up.sql.

UPDATE ai_tools
SET description = 'Search the web via Tavily or Brave (configurable). Results are cached for 30 minutes. Requires agent.external_search permission.',
    config = jsonb_set(
        config,
        '{description}',
        '"Search the web when internal knowledge is insufficient."'
    ),
    updated_at = now()
WHERE name = 'external.search';
