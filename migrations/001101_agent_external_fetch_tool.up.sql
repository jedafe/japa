-- 001101_agent_external_fetch_tool.up.sql
--
-- external.fetch — on-demand SSRF-safe web page fetch for the agent catalog
-- (§103). Complements external.search (which returns snippets) for the
-- "read this URL" case: the LLM can pull the full visible text of a page the
-- user pasted, an official website, or a result URL returned by search, so
-- answers and wiki/content updates are grounded in the fetched document.
--
-- New RBAC node agent.external_fetch, granted to super_admin, member, and
-- admin-group-role (mirroring the agent.external_search grants). The tool is
-- is_enabled in ai_tools; the executor's RBAC gate applies the node.
--
-- Ref: .jeos/sections/103-ai-agent-executor.md
--      .jeos/conventions/valkey-keys-catalog.md (jc:agent:fetch:{hash})

BEGIN;

-- ---- 1. Permission node ---------------------------------------------------

INSERT INTO permissions (key, description) VALUES
    ('agent.external_fetch', 'AI Agent — allow AI to fetch and read external web pages on user behalf')
ON CONFLICT (key) DO NOTHING;

-- super_admin + member (member-baseline like agent.external_search) +
-- admin-group-role (the staff group granted the agent/wiki nodes in 000791).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug IN ('super_admin', 'member', 'admin-group-role')
  AND p.key = 'agent.external_fetch'
ON CONFLICT DO NOTHING;

-- ---- 2. Catalog row --------------------------------------------------------

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('external.fetch', 'Fetch and read a specific web page on demand (SSRF-safe). Returns the page title and visible text. Use for a URL the user pasted, an official website, or a result URL from external.search when snippets are not enough. Results are cached per URL. Requires agent.external_fetch.', 'agent_knowledge', '{"function_name":"external.fetch","description":"Fetch and read a specific web page on demand. Returns the final URL (after redirects), page title, and visible text (HTML stripped). Use when the user gives a URL, points you at an official website, or you need the full contents of a page found via external.search. Do not use for search — call external.search to discover pages first.","handler":"external.fetch","parameters":{"type":"object","properties":{"url":{"type":"string","description":"Absolute http(s) URL to fetch"},"max_chars":{"type":"integer","description":"Max characters of page text to return (default from agent.external_fetch_max_chars setting, max 60000)"}},"required":["url"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'agent.external_fetch') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

-- ---- 3. Tunables (Rule 3 — no hardcoded limits) ----------------------------

INSERT INTO system_settings (key, value, is_public) VALUES
('agent.external_fetch_timeout_seconds',   '"15"',      false),
('agent.external_fetch_max_bytes',         '"2097152"', false),
('agent.external_fetch_max_chars',         '"16000"',   false),
('agent.external_fetch_cache_ttl_seconds', '"600"',     false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
