-- Migration 186: Stock photo provider configuration and media attribution columns.

-- Provider configuration table — stores enabled/disabled state and encrypted API keys.
CREATE TABLE stock_providers (
    provider_name     TEXT PRIMARY KEY,          -- 'unsplash', 'pexels', 'pixabay', 'openverse'
    enabled           BOOLEAN NOT NULL DEFAULT true,
    api_key_encrypted TEXT,                     -- AES-256-GCM v1:<nonce>:<ciphertext>, null for Openverse
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the four known providers. API keys are NULL initially — they get populated
-- from env vars at startup via the application seed logic.
INSERT INTO stock_providers (provider_name) VALUES
    ('unsplash'),
    ('pexels'),
    ('pixabay'),
    ('openverse')
ON CONFLICT (provider_name) DO NOTHING;

-- Stock photo attribution columns on media table.
ALTER TABLE media ADD COLUMN source_platform         TEXT;
ALTER TABLE media ADD COLUMN source_photographer      TEXT;
ALTER TABLE media ADD COLUMN source_photographer_url  TEXT;
ALTER TABLE media ADD COLUMN source_license           TEXT;
ALTER TABLE media ADD COLUMN source_license_url       TEXT;
ALTER TABLE media ADD COLUMN source_photo_id          TEXT;

COMMENT ON COLUMN media.source_platform        IS 'Stock provider name: unsplash, pexels, pixabay, openverse';
COMMENT ON COLUMN media.source_photographer     IS 'Photographer display name for attribution';
COMMENT ON COLUMN media.source_photographer_url IS 'Link to photographer profile page';
COMMENT ON COLUMN media.source_license          IS 'License identifier: cc0, cc-by, cc-by-sa, pixabay';
COMMENT ON COLUMN media.source_license_url      IS 'Link to license deed';
COMMENT ON COLUMN media.source_photo_id         IS 'Provider-specific photo ID for dedup';
