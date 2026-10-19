BEGIN;

DELETE FROM notification_types
WHERE key IN (
    'visa_timeline.prompt_update',
    'planning_group.checkin_reminder',
    'planning_group.mission_complete'
);

COMMIT;
