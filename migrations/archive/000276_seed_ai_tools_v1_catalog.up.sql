-- 000276 — Seed the V1 AI Tool Catalog (Section 20 — AI Core).
--
-- The ai_tools table (migration 000054) is the registry of callable AI
-- gateway tools, scoped per tenant. Section 20 itemises the V1 catalog
-- across three agents (Content, Moderation, Platform); the Analytics agent
-- deliberately has no tools in V1 (explicit v2/v3 placeholder, not an
-- oversight). Nothing was seeding these rows, so the Admin Panel
-- /ai/tools page (apps/admin, port 3001) rendered an empty list despite
-- the table, routes, handlers, and UI all being wired.
--
-- v2 ADRs (075–077) add Asynq orchestration tasks (studio:run_batch,
-- persona:run_activity, persona:dispatch_action) and the persona system,
-- but those are scheduled tasks that call the AI gateway via the existing
-- tools below — they are NOT additional ai_tools registry entries.
--
-- Ref: japaconnect-engineering/sections/20-ai-core.md (V1 Tool Catalog)
-- Ref: migration 000054_ai_tools.sql (table schema)
--
-- Idempotent: ON CONFLICT (tenant_id, name) DO NOTHING preserves any
-- admin-applied edits (description, is_enabled, required_permission,
-- config) on re-runs. Seed for every tenant so multi-tenant deployments
-- get the catalog too.

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission)
SELECT t.id, v.name, v.description, v.category, '{}'::jsonb, true, v.required_permission
FROM tenants t
CROSS JOIN (VALUES
    -- Content agent
    ('content.generate_draft',  'Generate a structured content draft from a topic brief and persona voice prompt.', 'content',  NULL),
    ('content.editor_assist',   'Inline editor assistance: improve, summarise, and rewrite selected text.',         'content',  'ai.editor.assist'),
    ('content.suggest_tags',    'Suggest tags for a content body.',                                                  'content',  NULL),
    -- Moderation agent
    ('moderation.flag_content',  'Flag content for review based on safety rules.',                                   'moderation', NULL),
    ('moderation.classify_severity', 'Classify the severity of a flagged content item.',                             'moderation', NULL),
    -- Platform agent
    ('platform.translate_string', 'Translate a string into a target language for i18n packs.',                       'platform', NULL)
) AS v(name, description, category, required_permission)
WHERE NOT EXISTS (
    SELECT 1 FROM ai_tools at
    WHERE at.tenant_id = t.id AND at.name = v.name
);
