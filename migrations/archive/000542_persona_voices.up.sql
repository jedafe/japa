-- 000542_persona_voices.up.sql
-- Adds a shared persona_voices table and a voice_id reference on ai_personas.
-- Migrates existing ai_personas.voice_prompt values into one persona_voices row
-- per persona (ADR-114).
--
-- Ref: .jeos/adr/ADR-114-shared-persona-voices-and-content-type-model-defaults.md
--      .jeos/sections/68-ai-personas.md

BEGIN;

CREATE TABLE IF NOT EXISTS persona_voices (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id),
    name         text NOT NULL,
    slug         text NOT NULL,
    description  text,
    voice_prompt text NOT NULL,
    is_system    boolean NOT NULL DEFAULT false,
    created_by   uuid NOT NULL REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

ALTER TABLE ai_personas
    ADD COLUMN IF NOT EXISTS voice_id uuid REFERENCES persona_voices(id) ON DELETE SET NULL;

-- One-time migration: create one persona_voices row per existing ai_personas row
-- that has a non-empty voice_prompt, link them, and clear the per-persona
-- voice_prompt so the shared voice becomes the source of truth.
WITH voices AS (
    INSERT INTO persona_voices (tenant_id, name, slug, voice_prompt, is_system, created_by)
    SELECT
        ap.tenant_id,
        COALESCE(pp.name || ' Voice', 'Persona Voice ' || ap.id::text),
        'persona-' || ap.id::text,
        ap.voice_prompt,
        false,
        ap.created_by
    FROM ai_personas ap
    JOIN professional_profiles pp ON pp.id = ap.profile_id
    WHERE ap.voice_prompt IS NOT NULL AND TRIM(ap.voice_prompt) <> ''
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id, slug
)
UPDATE ai_personas ap
SET
    voice_id = voices.id,
    voice_prompt = NULL
FROM voices
WHERE ap.id = (split_part(voices.slug, '-', 2))::uuid;

COMMIT;
