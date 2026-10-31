-- content_reports gains updated_at so non-terminal transitions
-- (pending → reviewing assignment/claim) have a timestamp the §22
-- moderation-history projection can place on the timeline — created_at
-- records the filing, resolved_at only stamps terminal states.
ALTER TABLE content_reports
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
