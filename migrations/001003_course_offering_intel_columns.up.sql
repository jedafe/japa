-- intel_record_id: provenance for the promoter + radar. editorial_overrides:
-- field-level set the promoter skips on re-crawl (human edit wins).
ALTER TABLE public.university_course_offerings
    ADD COLUMN intel_record_id uuid REFERENCES public.intel_records(id) ON DELETE SET NULL,
    ADD COLUMN editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL;

CREATE INDEX idx_course_offerings_intel_record
    ON public.university_course_offerings (intel_record_id)
    WHERE intel_record_id IS NOT NULL;
