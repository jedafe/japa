DROP INDEX IF EXISTS public.idx_course_offerings_intel_record;
ALTER TABLE public.university_course_offerings
    DROP COLUMN IF EXISTS intel_record_id,
    DROP COLUMN IF EXISTS editorial_overrides;
