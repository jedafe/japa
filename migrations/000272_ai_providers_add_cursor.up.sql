-- 000272 — Add 'cursor' to the ai_providers provider_type CHECK constraint.
--
-- The Cursor Cloud Agents API (Anysphere) was added as a dedicated provider
-- type in the AI Providers management page. This migration extends the CHECK
-- constraint to accept 'cursor' alongside the existing types.
--
-- Ref: internal/ai/cursor.go, internal/control/ai_providers.go

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'cursor', 'custom'));
