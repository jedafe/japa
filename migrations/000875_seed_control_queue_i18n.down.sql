-- 000875_seed_control_queue_i18n down: remove the seeded queue keys.
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'queue.action',
    'queue.archived_success',
    'queue.confirm_archive_title',
    'queue.confirm_purge_title',
    'queue.confirm_requeue_title',
    'queue.dlq_card',
    'queue.dlq_subtitle',
    'queue.error',
    'queue.last_failed',
    'queue.next_run',
    'queue.orphaned',
    'queue.payload',
    'queue.preview_note',
    'queue.purged_success',
    'queue.requeued_success',
    'queue.retries',
    'queue.retry_failed',
    'queue.retry_success',
    'queue.run_now',
    'queue.state_active',
    'queue.state_archived',
    'queue.state_completed',
    'queue.state_pending',
    'queue.state_retry',
    'queue.state_scheduled',
    'queue.task_id',
    'queue.task_type',
    'queue.view_details'
  );

COMMIT;
