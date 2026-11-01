-- 001014_seed_admin_jobs_i18n down: remove the seeded admin jobs i18n keys.
BEGIN;

DELETE FROM translations
WHERE locale IN ('en', 'fr')
  AND "key" IN (
    'common.deleteSelected', 'common.exportSelected', 'common.filters', 'common.processing',
    'jobs.application_deadline', 'jobs.application_url', 'jobs.body',
    'jobs.city', 'jobs.company', 'jobs.company_logo', 'jobs.company_website',
    'jobs.country', 'jobs.created', 'jobs.description', 'jobs.experience_level',
    'jobs.featured', 'jobs.job_function', 'jobs.job_role', 'jobs.job_type',
    'jobs.load_error', 'jobs.location', 'jobs.location_type',
    'jobs.relocation_assistance', 'jobs.salary', 'jobs.salary_currency',
    'jobs.salary_max', 'jobs.salary_min', 'jobs.salary_period', 'jobs.skills',
    'jobs.status', 'jobs.status_archived', 'jobs.status_draft',
    'jobs.status_in_review', 'jobs.status_scheduled',
    'jobs.status_published', 'jobs.summary', 'jobs.title', 'jobs.title_col',
    'jobs.toggle_error', 'jobs.toggle_feature', 'jobs.toggle_feature_confirm',
    'jobs.visa_sponsorship'
  );

COMMIT;
