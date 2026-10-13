-- 000847_agent_assist_tools.up.sql
-- AI consolidation: re-enable the six §20 V1 in-editor assist tools as real
-- agent executor tools + backfill feature.ai_content_gen.
--
-- 1. The v1 ai_tools rows for the /ai/assist/* REST endpoints were disabled
--    and blocked by migration 000788 because no executor handlers existed.
--    They now share one implementation with the REST handlers
--    (internal/ai/assist.go — AssistRunner, consumed by both the HTTP
--    handlers and the registered executor tools), so they are re-enabled
--    with real config JSONB. All six require ai.editor.assist, matching the
--    REST route gate (§103: "every tool call is authorized as if the user
--    made the API call directly"). chat.japabot, content.editor_assist
--    (v1 umbrella superseded by the six granular tools), and the
--    moderation.* rows stay disabled + blocked — they describe REST-only
--    surfaces, not executor tools.
--
-- 2. feature.ai_content_gen backfill: the /ai/assist/* endpoints move from
--    the legacy ai.content_assist_enabled system_setting to the
--    feature_flags reachability layer (same key as /ai/generate). The
--    legacy setting value is copied into the flag so operator choices are
--    preserved; the setting remains the deprecated nil-checker fallback.
--    Rename-mapping pattern mirrors 000811_task13_flag_backfill.
--
-- Ref: .jeos/sections/20-ai-core.md (Unified AI preflight + Assist gating amendments)
--      .jeos/sections/103-ai-agent-executor.md (Assist tools)

BEGIN;

-- ---- 1. Re-enable the six assist tools ------------------------------------

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Improve or rewrite text for clarity, grammar, and style, optionally per tone.',
    config = '{"function_name":"content.improve","description":"Improve or rewrite text for clarity, grammar, and style. Optionally specify a tone.","handler":"content.improve","parameters":{"type":"object","properties":{"text":{"type":"string","description":"The text to improve"},"tone":{"type":"string","description":"Writing tone (e.g. professional, casual, academic)"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"},"humanize":{"type":"boolean","description":"Override AI humanization"},"humanize_level":{"type":"string","description":"Humanization level: low|medium|high"}},"required":["text"],"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'content.improve';

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Summarize a body of text to a word budget.',
    config = '{"function_name":"content.summarize","description":"Summarize a body of text into a short excerpt.","handler":"content.summarize","parameters":{"type":"object","properties":{"text":{"type":"string","description":"The text to summarize"},"max_words":{"type":"integer","description":"Maximum words in the summary (default 100)"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"},"humanize":{"type":"boolean","description":"Override AI humanization"},"humanize_level":{"type":"string","description":"Humanization level: low|medium|high"}},"required":["text"],"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'content.summarize';

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Translate text into a target locale.',
    config = '{"function_name":"platform.translate_string","description":"Translate text into a target locale (e.g. fr, en, yo).","handler":"platform.translate_string","parameters":{"type":"object","properties":{"text":{"type":"string","description":"The text to translate"},"target_locale":{"type":"string","description":"Target locale code (e.g. fr)"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"},"humanize":{"type":"boolean","description":"Override AI humanization"},"humanize_level":{"type":"string","description":"Humanization level: low|medium|high"}},"required":["text","target_locale"],"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'platform.translate_string';

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Generate content tags from a title and body.',
    config = '{"function_name":"content.suggest_tags","description":"Generate relevant content tags for a title and/or body. Returns a JSON array of tag strings.","handler":"content.suggest_tags","parameters":{"type":"object","properties":{"title":{"type":"string","description":"Content title"},"text":{"type":"string","description":"Content body text"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"}},"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'content.suggest_tags';

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Generate Open Graph title/description/image-alt metadata.',
    config = '{"function_name":"content.generate_og","description":"Generate Open Graph metadata (og:title, og:description, og:image alt text) for a content item. Returns JSON.","handler":"content.generate_og","parameters":{"type":"object","properties":{"title":{"type":"string","description":"Content title"},"text":{"type":"string","description":"Content body text"},"content_type":{"type":"string","description":"Content type (default post)"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"}},"required":["title"],"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'content.generate_og';

UPDATE ai_tools SET
    is_enabled = true,
    required_permission = 'ai.editor.assist',
    description = 'Generate a structured content draft (title + body + field values) from a brief.',
    config = '{"function_name":"content.generate_draft","description":"Generate a structured content draft — title, body_html, and custom field values — from a topic brief, content type, and optional field schema. Returns JSON. Text generation only; use content.create_draft to persist a draft row for review.","handler":"content.generate_draft","parameters":{"type":"object","properties":{"content_type":{"type":"string","description":"Content type slug (e.g. blog_post, guide:relocation)"},"title":{"type":"string","description":"Working title or topic"},"existing_body":{"type":"string","description":"Existing body to improve/extend"},"instructions":{"type":"string","description":"Free-form generation instructions"},"tone":{"type":"string","description":"Writing tone"},"field_schema":{"type":"array","description":"Custom field schema to fill: [{key,type,label,options[]}]","items":{"type":"object"}},"options":{"type":"object","description":"Structure options: mode, framework (aida|pas), word/paragraph/heading counts, link counts, include_* flags"},"context":{"type":"object","description":"Extra context values for the brief"},"model":{"type":"string","description":"Optional model override"},"voice_id":{"type":"string","description":"Optional persona voice UUID"},"humanize":{"type":"boolean","description":"Override AI humanization"},"humanize_level":{"type":"string","description":"Humanization level: low|medium|high"}},"required":["content_type"],"additionalProperties":true},"reads_user_data":false,"writes_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
    updated_at = now()
WHERE name = 'content.generate_draft';

-- ---- 2. feature.ai_content_gen backfill ------------------------------------
-- The /ai/assist/* endpoints now resolve reachability via
-- feature.ai_content_gen (same flag as /ai/generate). Copy the legacy
-- ai.content_assist_enabled setting into the flag when an operator set it;
-- absent → registry default applies.

INSERT INTO feature_flags (key, enabled, updated_at)
SELECT 'feature.ai_content_gen', COALESCE((s.value #>> '{}')::boolean, true), now()
FROM system_settings s
WHERE s.key = 'ai.content_assist_enabled'
  AND jsonb_typeof(s.value) IN ('string','boolean')
ON CONFLICT (key) DO NOTHING;

COMMIT;
