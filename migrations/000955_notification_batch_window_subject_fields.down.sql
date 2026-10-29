ALTER TABLE notification_batch_windows
    DROP COLUMN IF EXISTS subject_title,
    DROP COLUMN IF EXISTS action_url;
