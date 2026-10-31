-- content_reports gains updated_at so non-terminal transitions
-- (pending → reviewing assignment/claim) have a timestamp the §22
-- moderation-history projection can place on the timeline — created_at
-- records the filing, resolved_at only stamps terminal states.
-- Backfill uses resolved_at (terminal rows) or created_at, NOT now(): a
-- naive now() default would stamp every pre-existing row with migration
-- time and make pending reports falsely read as "reopened"
-- (updated_at > created_at is the reopen signal).
ALTER TABLE content_reports
    ADD COLUMN updated_at timestamptz;

UPDATE content_reports
SET updated_at = COALESCE(resolved_at, created_at);

ALTER TABLE content_reports
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now();
