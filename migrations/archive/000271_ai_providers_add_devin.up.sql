-- 000270 — Add 'devin' to the ai_providers provider_type CHECK constraint.
--
-- The Devin agent API (Cognition) was added as a dedicated provider type
-- in the AI Providers management page. This migration extends the CHECK
-- constraint to accept 'devin' alongside the existing types.
--
-- Ref: internal/ai/devin.go, internal/control/ai_providers.go

ALTER TABLE ai_providers DROP CONSTRAINT IF EXISTS ai_providers_provider_type_check;
ALTER TABLE ai_providers ADD CONSTRAINT ai_providers_provider_type_check
    CHECK (provider_type IN ('xiaomi', 'openai', 'ollama', 'anthropic', 'devin', 'custom'));
