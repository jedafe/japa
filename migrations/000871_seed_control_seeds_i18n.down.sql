-- 000871_seed_control_seeds_i18n down: remove the seeded control seeds keys.
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'seeds.cron_link', 'seeds.empty', 'seeds.enqueue', 'seeds.enqueued',
    'seeds.enqueuing', 'seeds.intro', 'seeds.loading', 'seeds.param_required',
    'seeds.queue_link', 'seeds.select_default', 'seeds.select_placeholder',
    'seeds.title', 'seeds.track_prefix'
  );

COMMIT;
