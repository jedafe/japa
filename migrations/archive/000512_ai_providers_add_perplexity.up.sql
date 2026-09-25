-- 000512 — Add 'perplexity' to the ai_providers provider_type CHECK constraint.
--
-- Perplexity's sonar models expose an OpenAI-compatible Chat Completions API
-- and return a 'citations' array for grounded, source-attributed answers. This
-- migration extends the provider_type CHECK constraint so admins can register
-- a dedicated Perplexity provider alongside the existing types.
--
-- Ref: internal/ai/perplexity.go, internal/control/ai_providers.go

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'cursor', 'perplexity', 'custom'));
