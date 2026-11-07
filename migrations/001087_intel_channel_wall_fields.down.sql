ALTER TABLE intel_source_channels
    DROP COLUMN blocked_reason,
    DROP COLUMN last_wall_at,
    DROP COLUMN wall_seen_count;
