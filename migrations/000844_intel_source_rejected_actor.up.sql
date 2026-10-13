-- 000844_intel_source_rejected_actor.up.sql
--
-- Complete the intel_sources decision-actor lifecycle: proposed_by/
-- proposed_at and approved_by/approved_at already exist; a rejected
-- proposal previously stamped the reviewer into approved_by/approved_at,
-- which made a rejected row read as approved to anyone auditing the
-- columns. rejected_by/rejected_at records the decision actor with the
-- correct semantics; approved_* stays NULL on rejected rows.

BEGIN;

ALTER TABLE public.intel_sources
    ADD COLUMN rejected_by uuid,
    ADD COLUMN rejected_at timestamp with time zone,
    ADD CONSTRAINT intel_sources_rejected_by_fkey
        FOREIGN KEY (rejected_by) REFERENCES public.users(id) ON DELETE SET NULL;

COMMIT;
