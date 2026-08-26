-- 000557_update_ai_content_create_schema.up.sql
-- Section 103 (AI Agent Executor) — Guide content creation schema fix.
--
-- Fixes:
--   1. Adds guide_category to the content.create and content.create_draft
--      tool parameters so the LLM can set the correct guide category slug.
--   2. Clarifies the field_values parameter description: it is the free-form
--      contents.field_values JSONB, not structured custom field values.
--   3. Registers a new guide_categories.list tool so the agent can discover
--      admin-configured guide categories and their custom field group IDs.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

UPDATE ai_tools
SET config = '{
    "function_name": "content.create",
    "description": "Create new content with AI provenance metadata. Content is always created as draft.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Content title" },
            "body": { "type": "string", "description": "Content body (markdown)" },
            "content_type": { "type": "string", "description": "Content type (e.g. blog_post, guide, comparison, page, discussion, service_listing, story, achievement, job_listing, event). Call content_types.list to discover valid types and their custom field group entity classes." },
            "guide_category": { "type": "string", "description": "Guide category slug. Required when content_type is ''guide''. Call guide_categories.list to discover valid slugs and their custom field group IDs." },
            "author_id": { "type": "string", "description": "Author UUID (defaults to calling user)" },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "space_id": { "type": "string", "description": "Optional space" },
            "organization_id": { "type": "string", "description": "Optional organization UUID. Useful for job_listings and service_listings posted on behalf of an org (requires the author to be an owner/admin/editor of that org)." },
            "publish_at": { "type": "string", "description": "Optional RFC 3339 timestamp to schedule future publishing. For events this can represent the event start time." },
            "field_values": { "type": "object", "description": "Free-form JSONB for the contents.field_values column (e.g. page builder blocks, _ai_provenance). NOT for structured custom field values — use custom_field_values.bulk_set after content creation." }
        },
        "required": ["title", "content_type"]
    },
    "handler": "ContentCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb,
    description = 'Create new content. The agent injects _ai_provenance metadata into field_values for transparency. Content is always created as draft — the agent never publishes directly. Set guide_category when content_type is "guide".'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'content.create';

UPDATE ai_tools
SET config = '{
    "function_name": "content.create_draft",
    "description": "Create a draft content row for admin review (agent.draft_request path). Author is the AI persona; status is forced to draft; a HITL review is created.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Content title" },
            "body": { "type": "string", "description": "Content body (markdown), synthesized from external search results + RAG" },
            "content_type": { "type": "string", "description": "Content type (e.g. blog_post, guide, comparison, page, discussion, service_listing, story, achievement, job_listing, event). Call content_types.list to discover valid types and their custom field group entity classes." },
            "guide_category": { "type": "string", "description": "Guide category slug. Required when content_type is ''guide''. Call guide_categories.list to discover valid slugs and their custom field group IDs." },
            "author_id": { "type": "string", "description": "AI persona backing user UUID (resolved by the agent loop from agent.default_persona_id; the tool ignores any other value)" },
            "ai_persona_id": { "type": "string", "description": "The AI persona UUID (from agent.default_persona_id). Pass this so the draft records which persona authored it." },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "space_id": { "type": "string", "description": "Optional space" },
            "organization_id": { "type": "string", "description": "Optional organization UUID. Useful for job_listings and service_listings posted on behalf of an org (requires the author to be an owner/admin/editor of that org)." },
            "publish_at": { "type": "string", "description": "Optional RFC 3339 timestamp to schedule future publishing. For events this can represent the event start time." },
            "field_values": { "type": "object", "description": "Free-form JSONB for the contents.field_values column (e.g. page builder blocks, _ai_provenance). NOT for structured custom field values — use custom_field_values.bulk_set after content creation." },
            "original_prompt": { "type": "string", "description": "The original user prompt that triggered this draft (for provenance)" }
        },
        "required": ["title", "content_type", "body"]
    },
    "handler": "ContentCreateDraft",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb,
    description = 'Create a draft content row for admin review. Use this when pre-search found nothing and the user lacks content.create permission. The author is the AI persona (not the calling user); status is always draft. A HITL review is created. Set guide_category when content_type is "guide".'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'content.create_draft';

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'guide_categories.list',
 'List admin-configured guide categories and their custom field group IDs. Use before creating guide content so the correct guide_category and field group are known.',
 'agent_content',
 '{
    "function_name": "guide_categories.list",
    "description": "List admin-configured guide categories for the tenant. Returns slug, label, is_system, group_id, and group_slug. Use the group_id with custom_fields.list to discover the category''s field definitions.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "GuideCategoriesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}'::jsonb,
 true, NULL)
ON CONFLICT (tenant_id, name) DO UPDATE SET
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;

COMMIT;
