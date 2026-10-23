-- 000928_seed_roadmap_agent_tools.up.sql
--
-- Public-roadmap tools for the AI agent catalog (§103) — exposes the
-- §113 admin surface to the built-in agent and to MCP callers (§119),
-- enabling agent-driven changelog generation: an agent lists existing
-- items to dedupe, creates/patches an item, then set_status → shipped
-- stamps shipped_at, enqueues roadmap:ship_notify, and publishes the
-- entry to /roadmap/changelog.
--
-- RBAC: reuses the existing roadmap.* nodes — each tool is gated by the
-- same node as its /admin/roadmap/* REST counterpart. No new permission
-- keys are minted.

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
('roadmap.categories.list',
 'List roadmap categories (slug, name, color, sort order) for mapping an item to a category before create/update.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.categories.list",
    "description": "List roadmap categories.",
    "parameters": {
        "type": "object",
        "properties": {},
        "required": []
    },
    "handler": "RoadmapCategoriesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.items.list',
 'List roadmap items with optional status, search, and category filters (paginated). Use this to dedupe before creating a changelog entry.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.items.list",
    "description": "List roadmap items with optional status/search/category filters and pagination.",
    "parameters": {
        "type": "object",
        "properties": {
            "status":      { "type": "string", "description": "Optional status filter (pending, under_review, planned, in_progress, testing, shipped, declined, archived)" },
            "q":           { "type": "string", "description": "Optional title/slug search" },
            "category_id": { "type": "string", "description": "Optional category UUID filter" },
            "sort":        { "type": "string", "enum": ["recent", "votes", "title"], "description": "Sort order (default recent)" },
            "page":        { "type": "integer", "description": "Page number, 1-based (default 1)" },
            "limit":       { "type": "integer", "description": "Page size, max 100 (default 25)" },
            "include_deleted": { "type": "boolean", "description": "Include soft-deleted rows (default false)" }
        },
        "required": []
    },
    "handler": "RoadmapItemsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.item.get',
 'Read a single roadmap item by UUID — full markdown body, status, category, votes, shipped_at, feature_flag_key, and deleted_at.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.item.get",
    "description": "Read a single roadmap item by UUID.",
    "parameters": {
        "type": "object",
        "properties": {
            "id": { "type": "string", "description": "Roadmap item UUID" }
        },
        "required": ["id"]
    },
    "handler": "RoadmapItemGet",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.item.create',
 'Create a roadmap item. status="shipped" lands it directly on the public changelog and notifies voters; otherwise it enters the lifecycle at the given status.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.item.create",
    "description": "Create a roadmap item.",
    "parameters": {
        "type": "object",
        "properties": {
            "title":            { "type": "string", "description": "Item title (required)" },
            "slug":             { "type": "string", "description": "URL slug (optional — derived from title when omitted)" },
            "summary":          { "type": "string", "description": "Card preview / changelog line" },
            "body":             { "type": "string", "description": "Markdown body" },
            "category_id":      { "type": "string", "description": "Category UUID (see roadmap.categories.list)" },
            "status":           { "type": "string", "description": "Initial status (default under_review); shipped publishes to the changelog" },
            "sort_order":       { "type": "integer", "description": "Board column ordering (default 0)" },
            "is_flagship":      { "type": "boolean", "description": "Pin as the flagship now-building item" },
            "target_window":    { "type": "string", "description": "Free label, e.g. Q4 2026" },
            "feature_flag_key": { "type": "string", "description": "Link to a feature flag — status then derives from the flag rollout phase" }
        },
        "required": ["title"]
    },
    "handler": "RoadmapItemCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.create'),

('roadmap.item.update',
 'Patch a roadmap item — title, slug, summary, body, category, status, ordering, flagship, target window, feature flag link.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.item.update",
    "description": "Update a roadmap item.",
    "parameters": {
        "type": "object",
        "properties": {
            "id":               { "type": "string", "description": "Roadmap item UUID" },
            "title":            { "type": "string" },
            "slug":             { "type": "string" },
            "summary":          { "type": "string" },
            "body":             { "type": "string", "description": "Markdown body" },
            "category_id":      { "type": "string", "description": "Category UUID; empty string clears the category" },
            "sort_order":       { "type": "integer" },
            "is_flagship":      { "type": "boolean" },
            "target_window":    { "type": "string" },
            "feature_flag_key": { "type": "string", "description": "Empty string unlinks the flag" }
        },
        "required": ["id"]
    },
    "handler": "RoadmapItemUpdate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.edit'),

('roadmap.item.set_status',
 'Move a roadmap item through the lifecycle (e.g. in_progress → shipped). Shipping stamps shipped_at, publishes to /roadmap/changelog, and notifies voters. Flag-linked items reject with status_derived.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.item.set_status",
    "description": "Transition a roadmap item status.",
    "parameters": {
        "type": "object",
        "properties": {
            "id":     { "type": "string", "description": "Roadmap item UUID" },
            "status": { "type": "string", "enum": ["pending", "under_review", "planned", "in_progress", "testing", "shipped", "declined", "archived"], "description": "Target status" }
        },
        "required": ["id", "status"]
    },
    "handler": "RoadmapItemSetStatus",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.moderate')
ON CONFLICT (name) DO NOTHING;

COMMIT;
