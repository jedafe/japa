-- 000512 — Reverses 000512_ai_providers_add_perplexity.up.sql.
--
-- Removes 'perplexity' from the ai_providers provider_type CHECK constraint.
-- Any rows with provider_type='perplexity' must be deleted or updated before
-- this down migration can run.
--
-- Ref: internal/ai/perplexity.go, internal/control/ai_providers.go

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'cursor', 'custom'));
