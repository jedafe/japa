-- 000519 — Add 'groq' to the ai_providers provider_type CHECK constraint.
--
-- Groq exposes an OpenAI-compatible Chat Completions API. A dedicated
-- provider_type lets admins register Groq as a first-class provider and
-- lets the Control Panel show Groq-specific hints and presets.
--
-- Ref: internal/ai/groq.go, internal/control/ai_providers.go

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'groq', 'ollama', 'anthropic', 'devin', 'cursor', 'perplexity', 'custom'));
