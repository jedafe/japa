-- 000995_intel_domain_extended_fields.up.sql
-- Columns for the extended extraction fields added to the scholarship and
-- job schemas. Prior to this they were captured on intel_records.extracted
-- only; now they land on the projection rows so /tools surfaces can render
-- them. All nullable (except defaulted arrays) — existing rows stay valid.

BEGIN;

ALTER TABLE scholarships
  ADD COLUMN scholarship_type text,
  ADD COLUMN study_mode text,
  ADD COLUMN number_of_awards integer,
  ADD COLUMN application_fee text,
  ADD COLUMN renewable boolean,
  ADD COLUMN duration_years numeric,
  ADD COLUMN start_date date,
  ADD COLUMN open_to_international_students boolean,
  ADD COLUMN contact_email text;

ALTER TABLE intel_jobs
  ADD COLUMN visa_sponsorship boolean,
  ADD COLUMN relocation_assistance boolean,
  ADD COLUMN benefits text[] NOT NULL DEFAULT '{}',
  ADD COLUMN education_required text,
  ADD COLUMN languages_required text[] NOT NULL DEFAULT '{}',
  ADD COLUMN posted_date date,
  ADD COLUMN work_schedule text;

COMMIT;
