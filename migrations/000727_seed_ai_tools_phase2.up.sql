-- 000727_seed_ai_tools_phase2.up.sql
-- Agent Data API Phase 2 (spec 2026-09-24-agent-data-api-phase2-design) —
-- agent tool catalog entries for the feed, scam-alert, and semantic-search
-- reads surfaced over the §47 public API facade.
--
-- Registers:
--   - feeds.explore
--   - feeds.for_you
--   - scam.list
--   - scam.get
--   - search.semantic
--
-- These are surfaced on the MCP bridge (§119, ADR-116) via the agent tool
-- catalog so API-key callers can query the anonymous + personalized feeds,
-- confirmed scam alerts, and premium semantic search through the same
-- publicapi.Service read path as the /api/v1/public/* REST endpoints.
-- required_permission is NULL — the tools are read-only and agent.use gates
-- at the door; API-key callers are additionally restricted by the
-- apiKeyToolGates scope + tier map in internal/mcp (search.semantic mirrors
-- the REST RequireTier("pro") gate there).
--
-- Follows the de-tenanted ai_tools schema — migration 000668_final_de_tenant
-- dropped ai_tools.tenant_id and 000671 re-unique-indexed on (name) alone,
-- same as 000723.
--
-- Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md
--      .jeos/sections/103-ai-agent-executor.md (Tool Catalog Expansion)

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
(
    'feeds.explore',
    'Browse the public explore feeds. Optionally scope to a tag (tag) or a space (space_slug); otherwise returns the named explore feed (popular, latest, or rising — default popular). sort overrides the name mapping when valid (latest, top, rising, relevant).',
    'agent_data',
    '{
        "function_name": "feeds.explore",
        "description": "Browse the public explore feeds (popular, latest, rising), a tag feed, or a space feed.",
        "parameters": {
            "type": "object",
            "properties": {
                "name": {
                    "type": "string",
                    "description": "Explore feed name (default popular)",
                    "enum": ["popular", "latest", "rising"]
                },
                "space_slug": { "type": "string", "description": "Space slug — when set, returns that space feed instead" },
                "tag": { "type": "string", "description": "Tag slug — when set, returns the tag feed instead" },
                "sort": {
                    "type": "string",
                    "description": "Sort override",
                    "enum": ["latest", "top", "rising", "relevant"]
                },
                "limit": { "type": "integer", "description": "Max results (default 20, max 50)" },
                "offset": { "type": "integer", "description": "Offset for explore/space feed pagination (default 0)" },
                "cursor": { "type": "string", "description": "Opaque next_cursor for tag-feed pagination" }
            }
        },
        "handler": "FeedsExplore",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'feeds.for_you',
    'Read the calling user''s ranked for-you feed — the same personalized feed as GET /public/me/feed, resolved against the session user or API-key owner.',
    'agent_data',
    '{
        "function_name": "feeds.for_you",
        "description": "Read the calling user''s ranked for-you feed.",
        "parameters": {
            "type": "object",
            "properties": {
                "sort": {
                    "type": "string",
                    "description": "Sort mode",
                    "enum": ["latest", "top", "rising", "relevant"]
                },
                "limit": { "type": "integer", "description": "Max results (default 20, max 50)" },
                "offset": { "type": "integer", "description": "Offset for pagination (default 0)" }
            }
        },
        "handler": "FeedsForYou",
        "reads_user_data": true,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'scam.list',
    'List confirmed scam alerts. Optionally filter by destination space or scam type. Returns matching reports with total count.',
    'agent_data',
    '{
        "function_name": "scam.list",
        "description": "List confirmed scam alerts.",
        "parameters": {
            "type": "object",
            "properties": {
                "destination_space_id": { "type": "string", "description": "Destination space UUID to filter reports" },
                "scam_type": { "type": "string", "description": "Scam type filter (see scam-alerts schema endpoint for options)" },
                "limit": { "type": "integer", "description": "Max results (default 20, max 50)" },
                "offset": { "type": "integer", "description": "Offset for pagination (default 0)" }
            }
        },
        "handler": "ScamList",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'scam.get',
    'Read a single confirmed scam alert by ID.',
    'agent_data',
    '{
        "function_name": "scam.get",
        "description": "Read a single confirmed scam alert by ID.",
        "parameters": {
            "type": "object",
            "properties": {
                "id": { "type": "string", "description": "Scam alert UUID" }
            },
            "required": ["id"]
        },
        "handler": "ScamGet",
        "reads_user_data": false,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true,
    NULL
),
(
    'search.semantic',
    'Semantic (vector) search over indexed spaces and content. Premium — requires a pro subscription. Optionally constrain by language and space.',
    'agent_data',
    '{
        "function_name": "search.semantic",
        "description": "Semantic (vector) search over indexed spaces and content. Premium tool.",
        "parameters": {
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "Natural-language search query" },
                "lang": { "type": "string", "description": "Language code to bias retrieval (e.g. en, fr)" },
                "space": { "type": "string", "description": "Space slug to constrain results" },
                "limit": { "type": "integer", "description": "Max hits (server default applies when omitted)" }
            },
            "required": ["query"]
        },
        "handler": "SemanticSearch",
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
