-- 000278 — Register moderation.detect_spam in the AI tool registry.
--
-- The spam pipeline (internal/spam/pipeline.go) now has an AI classification
-- layer (checkAIClassification) that calls the AI gateway's "moderation"
-- category for semantic spam detection. This tool is the registry entry that
-- makes that layer visible and usage-trackable in the Admin Panel /ai/tools
-- page.
--
-- The moderation.ai_auto_flag_threshold setting (default 0.85) controls the
-- confidence level at which the AI score is treated as high-confidence.
--
-- Ref: japaconnect-engineering/sections/20-ai-core.md (V1 Tool Catalog)
-- Ref: japaconnect-engineering/sections/41-spam-content-safety.md
-- Ref: internal/spam/ai_classifier.go (AIClassifier adapter)
-- Ref: internal/spam/pipeline.go (checkAIClassification layer)

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission)
SELECT t.id, 'moderation.detect_spam',
       'AI-powered semantic spam classification via the moderation gateway category. Catches scam pitches, social engineering, and promotional flooding that rule-based layers miss.',
       'moderation', '{}'::jsonb, true, NULL
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM ai_tools at
    WHERE at.tenant_id = t.id AND at.name = 'moderation.detect_spam'
);
