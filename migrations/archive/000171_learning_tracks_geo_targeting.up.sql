-- Add geo_targeting to learning_tracks (ADR-064).
-- A track with a geo_space_id is relevant primarily to the country that space
-- belongs to. The search indexer walks the space hierarchy to resolve the
-- ISO country code and populates relevant_countries accordingly. NULL means
-- the track is globally relevant (the default).
ALTER TABLE learning_tracks
    ADD COLUMN IF NOT EXISTS geo_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_learning_tracks_geo_space
    ON learning_tracks (tenant_id, geo_space_id)
    WHERE geo_space_id IS NOT NULL;
