-- 001015_seed_admin_jobs_i18n_gap down: remove the gap-seeded keys.
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'common.filters', 'jobs.status_in_review', 'jobs.status_scheduled'
  );

COMMIT;
