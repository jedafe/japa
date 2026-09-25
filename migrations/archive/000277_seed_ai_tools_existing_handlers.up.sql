-- 000277 — Seed AI tools whose handlers already exist but were never registered.
--
-- Four AI gateway capabilities have working HTTP handlers and dispatch to the
-- AI gateway, but were absent from the ai_tools registry — so they were
-- invisible in the Admin Panel /ai/tools page and their usage was not
-- tracked in the usage tab.
--
--   content.generate_og  — HandleGenerateOG  (handlers.go:855, category content_generation)
--   content.summarize    — HandleSummarize   (handlers.go:689, category content_generation)
--   content.improve      — HandleImprove     (handlers.go:632, category content_generation)
--   chat.japabot         — HandleChat        (handlers.go:466, category chat)
--
-- Ref: japaconnect-engineering/sections/20-ai-core.md (V1 Tool Catalog)
-- Ref: migration 000054_ai_tools.sql (table schema)
--
-- Idempotent: ON CONFLICT (tenant_id, name) DO NOTHING preserves admin edits.

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission)
SELECT t.id, v.name, v.description, v.category, '{}'::jsonb, true, v.required_permission
FROM tenants t
CROSS JOIN (VALUES
    -- Content agent — handlers exist, were not registered
    ('content.generate_og', 'Generate Open Graph title and description from a content body.', 'content', NULL),
    ('content.summarize',    'Summarize a content body into a short excerpt.',                'content', NULL),
    ('content.improve',      'Improve or rewrite selected text per an editor instruction.',   'content', NULL),
    -- Chat agent — JapaBot conversational AI with RAG-grounded retrieval
    ('chat.japabot',         'Conversational AI assistant with RAG-grounded retrieval for immigration, visa, and finance questions.', 'chat', 'ai.context.use')
) AS v(name, description, category, required_permission)
WHERE NOT EXISTS (
    SELECT 1 FROM ai_tools at
    WHERE at.tenant_id = t.id AND at.name = v.name
);
