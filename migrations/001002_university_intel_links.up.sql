-- Links a university space (space_types.slug='education', level='university')
-- to the intel_record that created/last refreshed it, and carries the
-- field-level editorial_overrides set the promoter respects on re-crawl.
-- Mirrors the editorial_overrides convention on scholarships et al. without
-- adding intel plumbing columns to the shared spaces table.
CREATE TABLE public.university_intel_links (
    space_id            uuid PRIMARY KEY REFERENCES public.spaces(id) ON DELETE CASCADE,
    intel_record_id     uuid NOT NULL REFERENCES public.intel_records(id) ON DELETE CASCADE,
    editorial_overrides jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at          timestamptz DEFAULT now() NOT NULL,
    updated_at          timestamptz DEFAULT now() NOT NULL
);

CREATE INDEX idx_university_intel_links_record
    ON public.university_intel_links (intel_record_id);
