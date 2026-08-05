-- 000272 down — revert the CHECK constraint to exclude 'cursor'.
ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'custom'));
