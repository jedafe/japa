-- 000478_enable_tools_list_read_handlers.up.sql
-- Re-enables the tools.list and tools.read AI Agent tools now that they
-- have real backing handlers (agent_tools_registry.go). The
-- tools.create/update/assign_geo handlers remain disabled because they
-- redirect to the dedicated per-tool APIs.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
-- Ref: ADR-076 Amendment 1 (Approach B)

BEGIN;

UPDATE ai_tools
SET is_enabled = true,
    description = 'List available Japa tools (scholarships, visa_routes, housing_guides, etc.) with record counts. Optionally filter by category.'
WHERE name = 'tools.list';

UPDATE ai_tools
SET is_enabled = true,
    description = 'Read a single record from a tool table by tool_type and tool_id (UUID).'
WHERE name = 'tools.read';

COMMIT;
