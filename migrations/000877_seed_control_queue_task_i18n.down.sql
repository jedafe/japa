BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'queue.archive_task',
    'queue.delete_confirm',
    'queue.delete_task',
    'queue.delete_task_title',
    'queue.task_archived',
    'queue.task_deleted'
  );

COMMIT;
