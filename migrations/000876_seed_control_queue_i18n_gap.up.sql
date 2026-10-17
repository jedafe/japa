-- 000876_seed_control_queue_i18n_gap
-- Backfill queue.* keys present in apps/control locales but never seeded
-- to the translations table (pack returned null; UI relied on fallbacks).

BEGIN;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'en', 'queue.archive_all', 'Archive all'),
    (gen_random_uuid(), 'en', 'queue.bulk_failed', 'Queue operation failed: {error}'),
    (gen_random_uuid(), 'en', 'queue.confirm_archive', 'Archive all pending, scheduled, and retry tasks in {queue}? They move to the dead-letter queue and can be requeued.'),
    (gen_random_uuid(), 'en', 'queue.confirm_purge', 'Permanently delete all archived tasks in {queue}? This cannot be undone.'),
    (gen_random_uuid(), 'en', 'queue.confirm_requeue', 'Requeue all scheduled, retry, and archived tasks in {queue} for immediate processing?'),
    (gen_random_uuid(), 'en', 'queue.failed_to_load_tasks', 'Failed to load tasks'),
    (gen_random_uuid(), 'en', 'queue.no_tasks_in_state', 'No tasks in this state'),
    (gen_random_uuid(), 'en', 'queue.purge_archived', 'Purge archived'),
    (gen_random_uuid(), 'en', 'queue.requeue_all', 'Requeue all'),
    (gen_random_uuid(), 'fr', 'queue.archive_all', 'Tout archiver'),
    (gen_random_uuid(), 'fr', 'queue.bulk_failed', 'Échec de l''opération sur la file : {error}'),
    (gen_random_uuid(), 'fr', 'queue.confirm_archive', 'Archiver toutes les tâches en attente, planifiées et en nouvelle tentative dans {queue} ? Elles passent en file morte et peuvent être remises en file.'),
    (gen_random_uuid(), 'fr', 'queue.confirm_purge', 'Supprimer définitivement toutes les tâches archivées de {queue} ? Action irréversible.'),
    (gen_random_uuid(), 'fr', 'queue.confirm_requeue', 'Remettre en file toutes les tâches planifiées, en nouvelle tentative et archivées de {queue} pour un traitement immédiat ?'),
    (gen_random_uuid(), 'fr', 'queue.failed_to_load_tasks', 'Échec du chargement des tâches'),
    (gen_random_uuid(), 'fr', 'queue.no_tasks_in_state', 'Aucune tâche dans cet état'),
    (gen_random_uuid(), 'fr', 'queue.purge_archived', 'Purger les archives'),
    (gen_random_uuid(), 'fr', 'queue.requeue_all', 'Tout remettre en file')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

COMMIT;
