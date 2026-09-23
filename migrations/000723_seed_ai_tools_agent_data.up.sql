-- 000723_seed_ai_tools_agent_data.up.sql
-- Agent Data API v1 (spec 2026-09-23-agent-data-api) — agent tool catalog
-- entries for the read-only moat tools surfaced over the §47 public API
-- facade.
--
-- Registers:
--   - visa.search
--   - visa.get
--   - visa.compare
--   - university.get
--   - destination.brief
--
-- These are surfaced on the MCP bridge (§119, ADR-116) via the agent tool
-- catalog so API-key callers can query visa routes, universities, and the
-- aggregate destination brief through the same publicapi.Service read path
-- as the /api/v1/public/* REST endpoints. required_permission is NULL —
-- the tools are read-only and agent.use gates at the door; API-key callers
-- are additionally restricted by the apiKeyToolGates scope + tier map in
-- internal/mcp.
--
-- NOTE: the implementation plan specified tenant_id '00000000-...' per
-- migration 000388, but 000668_final_de_tenant dropped ai_tools.tenant_id
-- and 000671 re-unique-indexed on (name) alone. This seed follows the
-- de-tenanted schema (ToolRegistry.EnsureTool does the same).
--
-- Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
--      .jeos/sections/103-ai-agent-executor.md (Tool Catalog Expansion)

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
(
    'visa.search',
    'Search visa and immigration routes. Optionally filter by destination space, profession, education, and years of experience. Returns matching routes with total count.',
    'agent_data',
    '{
        "function_name": "visa.search",
        "description": "Search visa and immigration routes.",
        "parameters": {
            "type": "object",
            "properties": {
                "destination_space_id": { "type": "string", "description": "Destination space UUID to filter routes" },
                "profession": { "type": "string", "description": "Profession filter" },
                "education": { "type": "string", "description": "Education level filter" },
                "experience_years": { "type": "integer", "description": "Years of experience filter" },
                "limit": { "type": "integer", "description": "Max results (default 20, max 50)" },
                "page": { "type": "integer", "description": "Page number (default 1)" }
            }
        },
        "handler": "VisaSearch",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'visa.get',
    'Read a single visa route record by ID.',
    'agent_data',
    '{
        "function_name": "visa.get",
        "description": "Read a single visa route record by ID.",
        "parameters": {
            "type": "object",
            "properties": {
                "route_id": { "type": "string", "description": "Visa route UUID" }
            },
            "required": ["route_id"]
        },
        "handler": "VisaGet",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'visa.compare',
    'Compare up to 5 visa routes side by side by their IDs.',
    'agent_data',
    '{
        "function_name": "visa.compare",
        "description": "Compare up to 5 visa routes side by side.",
        "parameters": {
            "type": "object",
            "properties": {
                "route_ids": {
                    "type": "array",
                    "description": "Visa route UUIDs to compare (1-5)",
                    "items": { "type": "string" },
                    "minItems": 1,
                    "maxItems": 5
                }
            },
            "required": ["route_ids"]
        },
        "handler": "VisaCompare",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'university.get',
    'Read a single university record by its space slug.',
    'agent_data',
    '{
        "function_name": "university.get",
        "description": "Read a single university record by slug.",
        "parameters": {
            "type": "object",
            "properties": {
                "slug": { "type": "string", "description": "University space slug" }
            },
            "required": ["slug"]
        },
        "handler": "UniversityGet",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'destination.brief',
    'Get the aggregate destination brief for a space slug: visa routes, processing timelines, universities, scholarships, and scam alerts composed into one document. Omit include to get all sections.',
    'agent_data',
    '{
        "function_name": "destination.brief",
        "description": "Get the aggregate destination brief for a space slug.",
        "parameters": {
            "type": "object",
            "properties": {
                "slug": { "type": "string", "description": "Destination space slug" },
                "include": {
                    "type": "array",
                    "description": "Sections to include (default: all)",
                    "items": {
                        "type": "string",
                        "enum": ["visa", "timelines", "universities", "scholarships", "scam_alerts"]
                    }
                }
            },
            "required": ["slug"]
        },
        "handler": "DestinationBrief",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
)
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
