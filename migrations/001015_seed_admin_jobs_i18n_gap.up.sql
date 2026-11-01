-- 001015_seed_admin_jobs_i18n_gap
-- Gap seed for databases that applied 001014 before the common.filters and
-- jobs.status_in_review / jobs.status_scheduled keys were folded into it.
-- Idempotent ON CONFLICT upsert — safe to run on fresh databases too.

BEGIN;

INSERT INTO translations (id, locale, "key", value)
VALUES
    (gen_random_uuid(), 'en', 'common.filters', 'Filters'),
    (gen_random_uuid(), 'en', 'jobs.status_in_review', 'In Review'),
    (gen_random_uuid(), 'en', 'jobs.status_scheduled', 'Scheduled'),
    (gen_random_uuid(), 'fr', 'common.filters', 'Filtres'),
    (gen_random_uuid(), 'fr', 'jobs.status_in_review', 'En révision'),
    (gen_random_uuid(), 'fr', 'jobs.status_scheduled', 'Planifié')
ON CONFLICT (locale, "key") DO UPDATE SET value = EXCLUDED.value;

COMMIT;
