BEGIN;

ALTER TABLE scholarships
  DROP COLUMN IF EXISTS scholarship_type,
  DROP COLUMN IF EXISTS study_mode,
  DROP COLUMN IF EXISTS number_of_awards,
  DROP COLUMN IF EXISTS application_fee,
  DROP COLUMN IF EXISTS renewable,
  DROP COLUMN IF EXISTS duration_years,
  DROP COLUMN IF EXISTS start_date,
  DROP COLUMN IF EXISTS open_to_international_students,
  DROP COLUMN IF EXISTS contact_email;

ALTER TABLE intel_jobs
  DROP COLUMN IF EXISTS visa_sponsorship,
  DROP COLUMN IF EXISTS relocation_assistance,
  DROP COLUMN IF EXISTS benefits,
  DROP COLUMN IF EXISTS education_required,
  DROP COLUMN IF EXISTS languages_required,
  DROP COLUMN IF EXISTS posted_date,
  DROP COLUMN IF EXISTS work_schedule;

COMMIT;
