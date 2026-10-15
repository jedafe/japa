-- 000864_agent_comparison_list_tool.up.sql
-- comparison.list — the JCE comparison directory for the agent
-- (GET /jce/comparisons parity). The agent can now discover existing
-- comparisons (including draft stubs via explicit status filters) before
-- reading/filling them with comparison.read / comparison.fill_values /
-- comparison.promote.
--
-- Ungated, like the public REST route (OptionalAuth). The handler mirrors
-- the route's directory default — published/public/detailed when no
-- explicit filters are given. Results are listing metadata only; bodies
-- and field values stay behind the ai_exempt-gated comparison.read tool,
-- so the list cannot bypass the privacy gate in bulk.
--
-- Ref: .jeos/sections/103-ai-agent-executor.md (Comparison Engine)
--      .jeos/sections/30-generic-comparison-system.md

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('comparison.list', 'List comparisons in the JCE directory with optional type, status, visibility, depth, space, and text filters plus sorting and pagination. Returns listing metadata (id, slug, title, status, visibility, depth, counts) — use comparison.read for the full matrix. Defaults to published/public/detailed when no filters are given.', 'agent_comparison', '{"function_name":"comparison.list","description":"List comparisons in the JCE directory with optional type, status, visibility, depth, space, and text filters plus sorting and pagination. Returns listing metadata (id, slug, title, status, visibility, depth, counts) — use comparison.read for the full matrix. Defaults to published/public/detailed when no filters are given.","handler":"comparison.list","parameters":{"type":"object","properties":{"type_slug":{"type":"string","description":"Comparison type slug filter (see comparison.types.list)"},"space_id":{"type":"string","description":"Space UUID filter"},"status":{"type":"string","description":"Content status filter (\"all\" clears the published default)"},"visibility":{"type":"string","description":"Visibility filter (\"all\" clears the public default)"},"depth":{"type":"string","description":"Comparison depth filter","enum":["quick","detailed","all"]},"q":{"type":"string","description":"Search term"},"mine":{"type":"boolean","description":"List only the calling user''s comparisons"},"sort":{"type":"string","description":"Sort key (e.g. created_at, title, popularity)"},"page":{"type":"integer","description":"Page number (default 1)"},"limit":{"type":"integer","description":"Page size (default 20)"}},"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, NULL) ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

COMMIT;
