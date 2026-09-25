-- 000630_video_sources.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Canonical external video record for imports and uploads.

CREATE TABLE video_sources (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    author_user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type       text NOT NULL CHECK (source_type IN ('upload','youtube','tiktok','instagram_reel','vimeo','other')),
    source_url        text NOT NULL,
    external_id       text,
    title             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    category          text,
    thumbnail_url     text,
    thumbnail_media_id uuid REFERENCES media(id) ON DELETE SET NULL,
    duration_secs     int,
    status            text NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','takedown_requested','removed')),
    field_values      jsonb NOT NULL DEFAULT '{}',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, source_url)
);

CREATE UNIQUE INDEX idx_video_sources_external_id ON video_sources (tenant_id, source_type, external_id) WHERE external_id IS NOT NULL;
CREATE INDEX idx_video_sources_author ON video_sources (tenant_id, author_user_id, status, created_at DESC);
