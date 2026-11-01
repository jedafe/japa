-- 000993_source_post_count_renderable.down.sql
-- Reverts to the increment/decrement post_count semantics (published
-- intel_records only, no projection/video/comparison legs).

BEGIN;

DROP TRIGGER IF EXISTS trg_scholarships_post_count ON scholarships;
DROP TRIGGER IF EXISTS trg_intel_jobs_post_count ON intel_jobs;
DROP TRIGGER IF EXISTS trg_remittance_rates_post_count ON remittance_rates;
DROP TRIGGER IF EXISTS trg_wiki_pages_post_count ON wiki_pages;
DROP TRIGGER IF EXISTS trg_contents_post_count ON contents;
DROP TRIGGER IF EXISTS trg_video_sources_post_count ON video_sources;

DROP FUNCTION IF EXISTS trg_contents_recalc_source_post_count();
DROP FUNCTION IF EXISTS trg_video_recalc_source_post_count();
DROP FUNCTION IF EXISTS trg_projection_recalc_source_post_count();

-- Restore the original 000656 increment/decrement function.
CREATE OR REPLACE FUNCTION trg_intel_records_post_count_sync()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status = 'published' THEN
            UPDATE intel_sources SET post_count = post_count + 1 WHERE id = NEW.source_id;
        END IF;
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            IF NEW.status = 'published' AND OLD.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources SET post_count = post_count + 1 WHERE id = NEW.source_id;
            ELSIF OLD.status = 'published' AND NEW.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources SET post_count = GREATEST(0, post_count - 1) WHERE id = NEW.source_id;
            END IF;
        END IF;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.status = 'published' THEN
            UPDATE intel_sources SET post_count = GREATEST(0, post_count - 1) WHERE id = OLD.source_id;
        END IF;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP FUNCTION IF EXISTS jc_recalc_source_post_count(uuid);

UPDATE intel_sources
SET post_count = (
    SELECT COUNT(*) FROM intel_records
    WHERE intel_records.source_id = intel_sources.id
      AND intel_records.status = 'published'
);

COMMIT;
