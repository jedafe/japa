-- 000871_seed_control_seeds_i18n
-- Seed control-panel i18n keys for the Control → Seeds page (en + fr).
-- ON CONFLICT keeps any operator-edited values.

BEGIN;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'en', 'seeds.cron_link', 'Cron'),
    (gen_random_uuid(), 'en', 'seeds.empty', 'No seed operations are configured.'),
    (gen_random_uuid(), 'en', 'seeds.enqueue', 'Enqueue job'),
    (gen_random_uuid(), 'en', 'seeds.enqueued', 'Job enqueued.'),
    (gen_random_uuid(), 'en', 'seeds.enqueuing', 'Enqueuing…'),
    (gen_random_uuid(), 'en', 'seeds.intro', 'Trigger data seeding and enrichment jobs. Enqueued ops run in the worker — watch Queue / Cron for progress.'),
    (gen_random_uuid(), 'en', 'seeds.loading', 'Loading…'),
    (gen_random_uuid(), 'en', 'seeds.param_required', '{label} is required'),
    (gen_random_uuid(), 'en', 'seeds.queue_link', 'Queue'),
    (gen_random_uuid(), 'en', 'seeds.select_default', '(default)'),
    (gen_random_uuid(), 'en', 'seeds.select_placeholder', 'Select…'),
    (gen_random_uuid(), 'en', 'seeds.title', 'Seeds'),
    (gen_random_uuid(), 'en', 'seeds.track_prefix', 'Track progress under'),

    (gen_random_uuid(), 'fr', 'seeds.cron_link', 'Cron'),
    (gen_random_uuid(), 'fr', 'seeds.empty', 'Aucune opération de seed n''est configurée.'),
    (gen_random_uuid(), 'fr', 'seeds.enqueue', 'Mettre en file d''attente'),
    (gen_random_uuid(), 'fr', 'seeds.enqueued', 'Tâche mise en file d''attente.'),
    (gen_random_uuid(), 'fr', 'seeds.enqueuing', 'Mise en file d''attente…'),
    (gen_random_uuid(), 'fr', 'seeds.intro', 'Déclenchez les tâches de seed et d''enrichissement. Les opérations mises en file d''attente s''exécutent dans le worker — suivez la progression dans Queue / Cron.'),
    (gen_random_uuid(), 'fr', 'seeds.loading', 'Chargement…'),
    (gen_random_uuid(), 'fr', 'seeds.param_required', '{label} est requis'),
    (gen_random_uuid(), 'fr', 'seeds.queue_link', 'File d''attente'),
    (gen_random_uuid(), 'fr', 'seeds.select_default', '(par défaut)'),
    (gen_random_uuid(), 'fr', 'seeds.select_placeholder', 'Sélectionner…'),
    (gen_random_uuid(), 'fr', 'seeds.title', 'Seeds'),
    (gen_random_uuid(), 'fr', 'seeds.track_prefix', 'Suivre la progression dans')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

COMMIT;
