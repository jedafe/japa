ALTER TABLE intel_source_channels
    ADD COLUMN blocked_reason text,
    ADD COLUMN last_wall_at timestamptz,
    ADD COLUMN wall_seen_count integer NOT NULL DEFAULT 0;
