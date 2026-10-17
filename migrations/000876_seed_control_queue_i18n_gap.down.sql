-- 000876_seed_control_queue_i18n_gap down
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'queue.archive_all',
    'queue.bulk_failed',
    'queue.confirm_archive',
    'queue.confirm_purge',
    'queue.confirm_requeue',
    'queue.failed_to_load_tasks',
    'queue.no_tasks_in_state',
    'queue.purge_archived',
    'queue.requeue_all'
  );

COMMIT;
