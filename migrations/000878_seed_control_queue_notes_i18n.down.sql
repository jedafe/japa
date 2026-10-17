BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'queue.completed_note',
    'queue.failed_note'
  );

COMMIT;
