-- 000877_seed_control_queue_task_i18n
-- Seed per-task action i18n keys for the Queue Monitor detail modal.

BEGIN;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'en', 'queue.archive_task', 'Archive'),
    (gen_random_uuid(), 'en', 'queue.delete_confirm', 'Permanently delete task {id}? This cannot be undone.'),
    (gen_random_uuid(), 'en', 'queue.delete_task', 'Remove'),
    (gen_random_uuid(), 'en', 'queue.delete_task_title', 'Delete task'),
    (gen_random_uuid(), 'en', 'queue.task_archived', 'Task archived'),
    (gen_random_uuid(), 'en', 'queue.task_deleted', 'Task deleted'),
    (gen_random_uuid(), 'fr', 'queue.archive_task', 'Archiver'),
    (gen_random_uuid(), 'fr', 'queue.delete_confirm', 'Supprimer définitivement la tâche {id} ? Action irréversible.'),
    (gen_random_uuid(), 'fr', 'queue.delete_task', 'Supprimer'),
    (gen_random_uuid(), 'fr', 'queue.delete_task_title', 'Supprimer la tâche'),
    (gen_random_uuid(), 'fr', 'queue.task_archived', 'Tâche archivée'),
    (gen_random_uuid(), 'fr', 'queue.task_deleted', 'Tâche supprimée')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

COMMIT;
