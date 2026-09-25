-- 000557_update_ai_content_create_schema.down.sql
-- Reverts the content.create / content.create_draft / guide_categories.list
-- schema changes from 000557.

BEGIN;

UPDATE ai_tools
SET config = '{
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
}'::jsonb,
    description = 'Create new content. The agent injects _ai_provenance metadata into field_values for transparency. Content is always created as draft — the agent never publishes directly.'
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
            "content_type": { "type": "string", "description": "Content type (e.g. article, guide)" },
            "author_id": { "type": "string", "description": "AI persona backing user UUID (resolved by the agent loop from agent.default_persona_id; the tool ignores any other value)" },
            "ai_persona_id": { "type": "string", "description": "The AI persona UUID (from agent.default_persona_id). Pass this so the draft records which persona authored it." },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "space_id": { "type": "string", "description": "Optional space" },
            "field_values": { "type": "object", "description": "Custom field values" },
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
    description = 'Create a draft content row for admin review. Use this when pre-search found nothing and the user lacks content.create permission. The author is the AI persona (not the calling user); status is always draft. A HITL review is created so an admin can approve, edit, reject, or reassign the author before publishing.'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'content.create_draft';

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'guide_categories.list';

COMMIT;
