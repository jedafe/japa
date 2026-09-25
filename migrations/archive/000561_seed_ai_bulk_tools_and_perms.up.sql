-- 000561_seed_ai_bulk_tools_and_perms.up.sql
-- Section 103 (AI Agent Executor) — Bulk content tools + per-content-type
-- agent permission nodes.
--
-- 1. Adds `agent.content_type.*` permission nodes for every built-in content
--    type, so user groups can be granted the ability to create specific types
--    through the agent (or have that ability revoked).
-- 2. Seeds `agent.bulk_create_max_items` and `agent.bulk_publish_max_items`
--    system settings with a 50-item default.
-- 3. Registers `content.bulk_create` and `content.bulk_publish` in the
--    `ai_tools` catalog.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (RBAC Permissions)
-- Ref: japaconnect-engineering/sections/03-rbac.md (Amendment — AI Agent Content-Type Permission Nodes)
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

-- -------------------------------------------------------------------------
-- 1. Per-content-type agent permission nodes.
-- -------------------------------------------------------------------------

INSERT INTO permissions (key, description) VALUES
    ('agent.content_type.blog_post',       'AI Agent — create blog_post content'),
    ('agent.content_type.discussion',      'AI Agent — create discussion content'),
    ('agent.content_type.page',            'AI Agent — create page content'),
    ('agent.content_type.comparison',      'AI Agent — create comparison content'),
    ('agent.content_type.guide',           'AI Agent — create guide content'),
    ('agent.content_type.event',           'AI Agent — create event content'),
    ('agent.content_type.job_listing',     'AI Agent — create job_listing content'),
    ('agent.content_type.service_listing', 'AI Agent — create service_listing content'),
    ('agent.content_type.story',           'AI Agent — create story content'),
    ('agent.content_type.achievement',     'AI Agent — create achievement content')
ON CONFLICT (key) DO UPDATE
SET description = EXCLUDED.description;

-- Grant the per-type nodes to the default 'member' role so existing members
-- continue to be able to create any content type through the agent.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND r.slug = 'member'
  AND p.key LIKE 'agent.content_type.%'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Grant the per-type nodes to the default 'super_admin' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND r.slug = 'super_admin'
  AND p.key LIKE 'agent.content_type.%'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- -------------------------------------------------------------------------
-- 2. Bulk operation item limits.
-- -------------------------------------------------------------------------

INSERT INTO system_settings (tenant_id, key, value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'agent.bulk_create_max_items',  to_jsonb(50)),
    ('00000000-0000-0000-0000-000000000001', 'agent.bulk_publish_max_items', to_jsonb(50))
ON CONFLICT (tenant_id, key) DO UPDATE
SET value = EXCLUDED.value;

-- -------------------------------------------------------------------------
-- 3. Bulk content tools in ai_tools.
-- -------------------------------------------------------------------------

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'content.bulk_create',
 'Create multiple content rows in one call. Each item supports author_id, organization_id, publish_at, guide_category, and field_values. HITL-queued as a bulk operation.',
 'agent_content',
 '{
    "function_name": "content.bulk_create",
    "description": "Create multiple content rows in one call.",
    "parameters": {
        "type": "object",
        "properties": {
            "items": {
                "type": "array",
                "description": "Content items to create",
                "items": {
                    "type": "object",
                    "properties": {
                        "title": { "type": "string", "description": "Content title" },
                        "content_type": { "type": "string", "description": "Content type slug (e.g. blog_post, job_listing, guide)" },
                        "body": { "type": "string", "description": "Content body" },
                        "author_id": { "type": "string", "description": "Author UUID (defaults to calling user; requires content.assign_author to override)" },
                        "space_id": { "type": "string", "description": "Space UUID" },
                        "geo_space_id": { "type": "string", "description": "Geo space UUID" },
                        "organization_id": { "type": "string", "description": "Organization UUID (author must be an org owner/admin/editor)" },
                        "publish_at": { "type": "string", "description": "Publish timestamp (RFC 3339)" },
                        "guide_category": { "type": "string", "description": "Guide category slug (when content_type = guide)" },
                        "field_values": { "type": "object", "description": "Free-form JSONB for contents.field_values" }
                    },
                    "required": ["title", "content_type"]
                }
            }
        },
        "required": ["items"]
    },
    "handler": "ContentBulkCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb,
true, 'content.create')
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'content.bulk_publish',
 'Change the status of multiple content rows in one call. Defaults to "published". HITL-queued as a bulk operation.',
 'agent_content',
 '{
    "function_name": "content.bulk_publish",
    "description": "Publish or transition the status of multiple content rows in one call.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_ids": { "type": "array", "description": "Content UUIDs", "items": { "type": "string" } },
            "status": { "type": "string", "description": "Target status (default published)" }
        },
        "required": ["content_ids"]
    },
    "handler": "ContentBulkPublish",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb,
true, 'content.publish')
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;

COMMIT;
