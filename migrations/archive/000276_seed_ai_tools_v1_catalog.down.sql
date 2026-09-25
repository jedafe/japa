-- 000276 down — remove the seeded V1 AI Tool Catalog rows.
-- Only deletes the six canonical V1 tool names; any admin-created tools
-- are preserved.
DELETE FROM ai_tools
WHERE name IN (
    'content.generate_draft',
    'content.editor_assist',
    'content.suggest_tags',
    'moderation.flag_content',
    'moderation.classify_severity',
    'platform.translate_string'
);
