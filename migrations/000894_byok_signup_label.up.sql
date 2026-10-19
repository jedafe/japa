-- signup_label customizes the "Get an API key →" affiliate CTA per provider
-- (e.g. "Get $5 credit at Groq →"). Empty falls back to the generic label.
ALTER TABLE ai_byok_providers ADD COLUMN signup_label text NOT NULL DEFAULT '';
