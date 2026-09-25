-- 000550_seed_campaign_agent_tools.up.sql
-- Section 104 (AI Content Campaigns) — Agent tool catalog entries.
--
-- Registers:
--   - personas.create
--   - content.campaign.create
--
-- Ref: .jeos/sections/104-content-campaigns.md

BEGIN;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
(
    '00000000-0000-0000-0000-000000000001',
    'personas.create',
    'Create one or more AI personas. Each persona receives a name, slug, tagline, voice prompt, and activity policy. Use when the campaign plan needs additional personas.',
    'agent_persona',
    '{
        "function_name": "personas.create",
        "description": "Create one or more AI personas with complete identities.",
        "parameters": {
            "type": "object",
            "properties": {
                "personas": {
                    "type": "array",
                    "description": "List of persona definitions to create",
                    "items": {
                        "type": "object",
                        "properties": {
                            "name": { "type": "string", "description": "Display name of the persona" },
                            "slug": { "type": "string", "description": "URL-safe unique slug" },
                            "tagline": { "type": "string", "description": "Short professional tagline" },
                            "voice_prompt": { "type": "string", "description": "Voice and tone guidance for content generation" },
                            "role_focus": { "type": "string", "description": "Primary role focus (e.g. immigration_lawyer, travel_blogger)" }
                        },
                        "required": ["name", "slug", "tagline", "voice_prompt"]
                    }
                }
            },
            "required": ["personas"]
        },
        "handler": "PersonasCreate",
        "reads_user_data": false,
        "writes_user_data": true,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'persona.manage'
),
(
    '00000000-0000-0000-0000-000000000001',
    'content.campaign.create',
    'Plan and optionally run a multi-piece content campaign. Expands the user brief into the requested number of unique content items, auto-provisions missing personas, assigns one distinct persona to each item, and dispatches them through the Studio review pipeline. Returns the campaign summary and status.',
    'agent_content',
    '{
        "function_name": "content.campaign.create",
        "description": "Create and optionally run a multi-piece content campaign with one persona per item.",
        "parameters": {
            "type": "object",
            "properties": {
                "prompt": { "type": "string", "description": "High-level campaign brief" },
                "count": { "type": "integer", "description": "Number of content pieces to generate (1-50, default 1)" },
                "content_type": { "type": "string", "description": "Optional content type (blog, story, guide, discussion, reply); omit to let the planner infer mixed types" },
                "persona_ids": { "type": "array", "items": { "type": "string" }, "description": "Optional list of persona UUIDs to use; omit for auto-assignment" },
                "auto_provision_personas": { "type": "boolean", "description": "Create missing personas automatically (default true)" },
                "auto_assign": { "type": "boolean", "description": "Allow AI to assign personas (default true)" },
                "target_space_id": { "type": "string", "description": "Optional target space UUID" },
                "target_tags": { "type": "array", "items": { "type": "string" }, "description": "Optional tag names" },
                "run_immediately": { "type": "boolean", "description": "Run the campaign after planning (default false)" }
            },
            "required": ["prompt"]
        },
        "handler": "ContentCampaignCreate",
        "reads_user_data": false,
        "writes_user_data": true,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    'campaign.create'
)
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
