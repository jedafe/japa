-- Section 34: Media System
-- Media uploads, storage, and management

CREATE TABLE media (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    uploader_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    original_filename text NOT NULL,
    mime_type         text NOT NULL,
    file_size_bytes   bigint NOT NULL,
    width_px          int,
    height_px         int,
    duration_secs     numeric,             -- for video/audio
    storage_path      text NOT NULL,       -- MinIO object key
    storage_bucket    text NOT NULL DEFAULT 'media',
    alt_text          text,
    caption           text,
    confirmed         boolean NOT NULL DEFAULT false,
    deleted_at        timestamptz,         -- soft delete
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_media_uploader ON media (tenant_id, uploader_user_id, created_at DESC);
CREATE INDEX idx_media_type ON media (tenant_id, mime_type, confirmed, deleted_at);
CREATE INDEX idx_media_tenant ON media (tenant_id, created_at DESC) WHERE deleted_at IS NULL;
