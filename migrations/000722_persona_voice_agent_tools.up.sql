-- 000722_persona_voice_agent_tools.up.sql
-- Section 68 (AI Personas) + ADR-114 (Shared Persona Voices) — Agent tool
-- catalog entries for read-only persona/voice discovery.
--
-- Registers:
--   - persona_voices.list
--   - persona_voices.read
--   - personas.list
--   - personas.read
--
-- These are surfaced on the MCP bridge (§119, ADR-116) via the agent tool
-- catalog so external agents can discover the shared voice/tone catalog and
-- pick an ai_persona_id before calling content.create_draft.
--
-- NOTE: 000668_final_de_tenant dropped ai_tools.tenant_id and 000671
-- re-unique-indexed on (name) alone — this seed follows the de-tenanted
-- schema (no tenant_id column).
--
-- Ref: .jeos/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
--      .jeos/adr/ADR-114-shared-persona-voices-and-content-type-model-defaults.md

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
(
    'persona_voices.list',
    'List the shared persona voices (tone catalog). Returns id, name, slug, description and is_system for each voice; pass include_prompts=true to also return each voice_prompt. Use persona_voices.read to fetch a single voice.',
    'agent_persona',
    '{
        "function_name": "persona_voices.list",
        "description": "List the shared persona voices (tone catalog).",
        "parameters": {
            "type": "object",
            "properties": {
                "limit": { "type": "integer", "description": "Max results (default 50, max 200)" },
                "offset": { "type": "integer", "description": "Pagination offset (default 0)" },
                "include_prompts": { "type": "boolean", "description": "Include each voice''s full voice_prompt in the result (default false)" }
            }
        },
        "handler": "PersonaVoicesList",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'persona.view'
),
(
    'persona_voices.read',
    'Read a single shared persona voice by id or slug, including the full voice_prompt used as tone guidance for content generation.',
    'agent_persona',
    '{
        "function_name": "persona_voices.read",
        "description": "Read a single shared persona voice including its voice_prompt.",
        "parameters": {
            "type": "object",
            "properties": {
                "id": { "type": "string", "description": "Voice UUID" },
                "slug": { "type": "string", "description": "Voice slug (used when id is not provided)" }
            }
        },
        "handler": "PersonaVoicesRead",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'persona.view'
),
(
    'personas.list',
    'List AI personas with their status, linked voice, and activity stats. Use the returned id as ai_persona_id when calling content.create_draft.',
    'agent_persona',
    '{
        "function_name": "personas.list",
        "description": "List AI personas with voice and activity info.",
        "parameters": {
            "type": "object",
            "properties": {
                "status": { "type": "string", "description": "Filter by status (active|paused|retired); empty = all" },
                "q": { "type": "string", "description": "Search by display name or backing username" },
                "sort": { "type": "string", "description": "Sort key: name|created_at|total_actions (default created_at)" },
                "dir": { "type": "string", "description": "Sort direction: asc|desc (default desc)" },
                "limit": { "type": "integer", "description": "Max results (default 50, max 100)" },
                "offset": { "type": "integer", "description": "Pagination offset (default 0)" }
            }
        },
        "handler": "PersonasList",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'persona.view'
),
(
    'personas.read',
    'Read a single AI persona by id, including the resolved voice_prompt (persona override wins over the shared voice) and activity_policy.',
    'agent_persona',
    '{
        "function_name": "personas.read",
        "description": "Read a single AI persona including resolved voice_prompt and activity_policy.",
        "parameters": {
            "type": "object",
            "properties": {
                "id": { "type": "string", "description": "Persona UUID" }
            },
            "required": ["id"]
        },
        "handler": "PersonasRead",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'persona.view'
)
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
