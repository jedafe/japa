-- 000632_video_import_jobs.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Bulk YouTube channel/playlist/keyword import job tracking.

CREATE TABLE video_import_jobs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id               uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type           text NOT NULL CHECK (source_type IN ('youtube_channel','youtube_playlist','youtube_search')),
    source_url            text,
    query                 text,
    target_author_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    status                text NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','processing','completed','failed','partial')),
    total_items           int NOT NULL DEFAULT 0,
    imported_count        int NOT NULL DEFAULT 0,
    failed_count          int NOT NULL DEFAULT 0,
    error_log             jsonb NOT NULL DEFAULT '[]',
    created_at            timestamptz NOT NULL DEFAULT now(),
    completed_at          timestamptz
);

CREATE INDEX idx_video_import_jobs_user ON video_import_jobs (tenant_id, user_id, status);
