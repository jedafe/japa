-- 000518 — Reverts 000518_add_groq_provider_type.up.sql.
--
-- Removes 'groq' from the ai_providers provider_type CHECK constraint.
-- Any rows with provider_type='groq' must be deleted or updated before this
-- down migration can run.

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'cursor', 'perplexity', 'custom'));
