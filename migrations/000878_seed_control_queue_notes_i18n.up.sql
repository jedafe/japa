-- 000878_seed_control_queue_notes_i18n
BEGIN;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'en', 'queue.completed_note', 'Completed tasks are not retained — only the 24h counter tracks them.'),
    (gen_random_uuid(), 'en', 'queue.failed_note', 'Failed tasks appear here while retrying or once exhausted. The 24h counter tracks failure events, not live tasks.'),
    (gen_random_uuid(), 'fr', 'queue.completed_note', 'Les tâches terminées ne sont pas conservées — seul le compteur 24h les suit.'),
    (gen_random_uuid(), 'fr', 'queue.failed_note', 'Les tâches échouées apparaissent ici en nouvelle tentative ou une fois épuisées. Le compteur 24h suit les événements d''échec, pas les tâches actives.')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

COMMIT;
