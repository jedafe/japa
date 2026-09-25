-- 000388_seed_ai_agent_tools_v1.up.sql
-- Section 103 (AI Agent Executor) — Phase 4 Core Tool Catalog.
--
-- Seeds the first 10 agent tools into the ai_tools table. Each row's
-- config JSONB contains the function schema (parameters), the handler
-- reference string (the Go handler registered in ToolHandlerRegistry),
-- and the behavioral flags (requires_hitl, reads_user_data,
-- writes_user_data, is_blocked) that drive the executor pipeline.
--
-- The required_permission column holds the RBAC node the executor's
-- RBAC gate checks (Phase 3, executor.go step 3). Tools with no
-- required_permission are available to every user who holds agent.use
-- (the agent loop checks agent.use at the door; the catalog filter is
-- per-tool).
--
-- NOTE: ROADMAP-AI-AGENT.md named this migration 000371, but 000371 was
-- already used (learning_certificates_unique_user_track). Latest applied
-- is 000377. 000378 collided with calendar_schema_fixes, so this migration
-- uses 000388.
-- Flagged per AGENTS.md Rule 1 (section file is source of truth;
-- numbering follows the runner).
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, task 4.11)
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- =====================================================================
-- Content tools (tasks 4.01-4.04)
-- =====================================================================

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'content.search',
 'Search site content by query. Returns matching content items with title, slug, status, and geo scope. When geo_space_id is provided, results are scoped to that space. Exempt content is filtered post-query.',
 'agent_content',
 '{
    "function_name": "content.search",
    "description": "Search site content by query. Returns matching content items.",
    "parameters": {
        "type": "object",
        "properties": {
            "query": { "type": "string", "description": "Search query" },
            "geo_space_id": { "type": "string", "description": "Optional geo scope (space UUID)" },
            "limit": { "type": "integer", "description": "Max results (default 10, max 50)" }
        },
        "required": ["query"]
    },
    "handler": "ContentSearch",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'content.read',
 'Read a single content record by ID, including its non-exempt custom field values. Content marked ai_exempt is blocked by the privacy gate.',
 'agent_content',
 '{
    "function_name": "content.read",
    "description": "Read a content record and its custom field values.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Content UUID" }
        },
        "required": ["content_id"]
    },
    "handler": "ContentRead",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'content.create',
 'Create new content. The agent injects _ai_provenance metadata into field_values for transparency. Content is always created as draft — the agent never publishes directly.',
 'agent_content',
 '{
    "function_name": "content.create",
    "description": "Create new content with AI provenance metadata.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Content title" },
            "body": { "type": "string", "description": "Content body (markdown)" },
            "content_type": { "type": "string", "description": "Content type (e.g. article, guide, comparison)" },
            "author_id": { "type": "string", "description": "Author UUID (defaults to calling user)" },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "space_id": { "type": "string", "description": "Optional space" },
            "field_values": { "type": "object", "description": "Custom field values" }
        },
        "required": ["title", "content_type"]
    },
    "handler": "ContentCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'content.create'),

('00000000-0000-0000-0000-000000000001', 'content.update',
 'Update an existing content record. Only the fields provided in the arguments are changed. Content marked ai_exempt is blocked.',
 'agent_content',
 '{
    "function_name": "content.update",
    "description": "Update an existing content record.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Content UUID" },
            "title": { "type": "string", "description": "New title" },
            "body": { "type": "string", "description": "New body" },
            "slug": { "type": "string", "description": "New slug" },
            "status": { "type": "string", "description": "New status" },
            "geo_space_id": { "type": "string", "description": "New geo scope" },
            "field_values": { "type": "object", "description": "Updated field values" }
        },
        "required": ["content_id"]
    },
    "handler": "ContentUpdate",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'content.update'),

-- =====================================================================
-- Custom field tools (tasks 4.05-4.08)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'custom_fields.list',
 'List field definitions for a custom field group. Returns field key, label, type, and validation flags.',
 'agent_fields',
 '{
    "function_name": "custom_fields.list",
    "description": "List custom field definitions for a group.",
    "parameters": {
        "type": "object",
        "properties": {
            "group_id": { "type": "string", "description": "Custom field group UUID" }
        },
        "required": ["group_id"]
    },
    "handler": "CustomFieldsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'custom_field_values.read',
 'Read custom field values for an entity. Values marked ai_exempt are filtered out.',
 'agent_fields',
 '{
    "function_name": "custom_field_values.read",
    "description": "Read custom field values for an entity (privacy-filtered).",
    "parameters": {
        "type": "object",
        "properties": {
            "entity_type": { "type": "string", "description": "Entity type (content, user, organization)" },
            "entity_id": { "type": "string", "description": "Entity UUID" }
        },
        "required": ["entity_type", "entity_id"]
    },
    "handler": "CFVRead",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'custom_field_values.set',
 'Set a single custom field value. The target entity is privacy-checked before the write.',
 'agent_fields',
 '{
    "function_name": "custom_field_values.set",
    "description": "Set a single custom field value.",
    "parameters": {
        "type": "object",
        "properties": {
            "field_id": { "type": "string", "description": "Field definition UUID" },
            "entity_type": { "type": "string", "description": "Entity type" },
            "entity_id": { "type": "string", "description": "Entity UUID" },
            "value_text": { "type": "string", "description": "Text value" },
            "value_bool": { "type": "boolean", "description": "Boolean value" },
            "value_date": { "type": "string", "description": "Date value (RFC 3339)" },
            "value_json": { "type": "object", "description": "JSON value" }
        },
        "required": ["field_id", "entity_type", "entity_id"]
    },
    "handler": "CFVSet",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'content.update'),

('00000000-0000-0000-0000-000000000001', 'custom_field_values.bulk_set',
 'Set multiple custom field values for a single entity in one call. Per-field status is reported. Bulk operations are HITL-queued.',
 'agent_fields',
 '{
    "function_name": "custom_field_values.bulk_set",
    "description": "Set multiple custom field values for an entity.",
    "parameters": {
        "type": "object",
        "properties": {
            "entity_type": { "type": "string", "description": "Entity type" },
            "entity_id": { "type": "string", "description": "Entity UUID" },
            "values": {
                "type": "array",
                "description": "Field value entries",
                "items": {
                    "type": "object",
                    "properties": {
                        "field_id": { "type": "string", "description": "Field definition UUID" },
                        "value_text": { "type": "string" },
                        "value_bool": { "type": "boolean" },
                        "value_date": { "type": "string" },
                        "value_json": { "type": "object" }
                    },
                    "required": ["field_id"]
                }
            }
        },
        "required": ["entity_type", "entity_id", "values"]
    },
    "handler": "CFVBulkSet",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'content.update'),

-- =====================================================================
-- Knowledge tools (tasks 4.09-4.10)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'rag.search',
 'Search the knowledge base via vector retrieval. When geo_space_id is provided, results are geo-filtered. Exempt content is filtered post-retrieval.',
 'agent_knowledge',
 '{
    "function_name": "rag.search",
    "description": "Search the knowledge base (geo-filtered vector retrieval).",
    "parameters": {
        "type": "object",
        "properties": {
            "query": { "type": "string", "description": "Search query" },
            "language": { "type": "string", "description": "Language code (default: en)" },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "limit": { "type": "integer", "description": "Max results" }
        },
        "required": ["query"]
    },
    "handler": "RAGSearch",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'external.search',
 'Search the web via Tavily or Brave (configurable). Results are cached for 30 minutes. Requires agent.external_search permission.',
 'agent_knowledge',
 '{
    "function_name": "external.search",
    "description": "Search the web when internal knowledge is insufficient.",
    "parameters": {
        "type": "object",
        "properties": {
            "query": { "type": "string", "description": "Web search query" },
            "limit": { "type": "integer", "description": "Max results (default 5, max 20)" }
        },
        "required": ["query"]
    },
    "handler": "ExternalSearch",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'agent.external_search')
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
