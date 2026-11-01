-- 000993_source_post_count_renderable.up.sql
-- post_count previously counted every intel_records row in 'published'
-- status, but the public source feed (ListIntelRecordsBySource) only renders
-- records whose domain projection row still exists, and additionally merges
-- video_sources and comparison contents items. The counter could drift both
-- ways: published records with deleted/missing projections inflated it (a
-- source showing "8 posts" over an empty feed) while video/comparison-backed
-- items never counted at all.
--
-- This migration replaces the increment/decrement bookkeeping with a
-- per-source recount that mirrors the feed's renderability rules exactly,
-- and adds triggers on every table that affects the count:
--   - intel_records (status/entity_type/source_id changes) — existing trigger
--     now recalcs instead of incrementing
--   - scholarships, intel_jobs, remittance_rates, wiki_pages (projection
--     rows linked by intel_record_id)
--   - contents (both linkages: intel_record_id projections AND
--     intel_source_id comparison items)
--   - video_sources (source_id + status='active')

BEGIN;

CREATE OR REPLACE FUNCTION jc_recalc_source_post_count(p_source_id uuid) RETURNS void AS $$
BEGIN
    UPDATE intel_sources SET post_count = (
        -- published intel_records whose backing projection exists (the
        -- feed's EXISTS filter in ListIntelRecordsBySource)
        (SELECT COUNT(*) FROM intel_records ir
          WHERE ir.source_id = p_source_id AND ir.status = 'published' AND (
               (ir.entity_type = 'scholarship' AND EXISTS (SELECT 1 FROM scholarships s WHERE s.intel_record_id = ir.id))
            OR (ir.entity_type = 'job' AND EXISTS (SELECT 1 FROM intel_jobs j WHERE j.intel_record_id = ir.id))
            OR (ir.entity_type = 'remittance' AND EXISTS (SELECT 1 FROM remittance_rates rr WHERE rr.intel_record_id = ir.id))
            OR (ir.entity_type IN ('news','culture_lifestyle','guide','blog','article')
                AND EXISTS (SELECT 1 FROM contents bc WHERE bc.intel_record_id = ir.id AND bc.deleted_at IS NULL))
            OR (ir.entity_type = 'wiki_page' AND EXISTS (SELECT 1 FROM wiki_pages w WHERE w.intel_record_id = ir.id))
          ))
        -- video feed leg (ListVideoSourceItemsBySourceID)
        + (SELECT COUNT(*) FROM video_sources vs WHERE vs.source_id = p_source_id AND vs.status = 'active')
        -- comparison feed leg (ListComparisonContentsBySourceID)
        + (SELECT COUNT(*) FROM contents c
            WHERE c.intel_source_id = p_source_id
              AND c.content_type = 'comparison'
              AND c.deleted_at IS NULL
              AND c.status = 'published'
              AND c.visibility = 'public')
    ) WHERE id = p_source_id;
END;
$$ LANGUAGE plpgsql;

-- intel_records: recount the affected source(s) instead of incrementing.
CREATE OR REPLACE FUNCTION trg_intel_records_post_count_sync() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM jc_recalc_source_post_count(OLD.source_id);
        RETURN OLD;
    END IF;
    PERFORM jc_recalc_source_post_count(NEW.source_id);
    IF TG_OP = 'UPDATE' AND OLD.source_id IS DISTINCT FROM NEW.source_id THEN
        PERFORM jc_recalc_source_post_count(OLD.source_id);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Pure projection tables (linked to a record by intel_record_id).
CREATE OR REPLACE FUNCTION trg_projection_recalc_source_post_count() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.intel_record_id IS NOT NULL THEN
            PERFORM jc_recalc_source_post_count((SELECT source_id FROM intel_records WHERE id = OLD.intel_record_id));
        END IF;
        RETURN OLD;
    END IF;
    IF NEW.intel_record_id IS NOT NULL THEN
        PERFORM jc_recalc_source_post_count((SELECT source_id FROM intel_records WHERE id = NEW.intel_record_id));
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.intel_record_id IS DISTINCT FROM NEW.intel_record_id AND OLD.intel_record_id IS NOT NULL THEN
        PERFORM jc_recalc_source_post_count((SELECT source_id FROM intel_records WHERE id = OLD.intel_record_id));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- contents carries both linkages: intel_record_id projections (news/guide/
-- blog/article render leg) and intel_source_id comparison items.
CREATE OR REPLACE FUNCTION trg_contents_recalc_source_post_count() RETURNS trigger AS $$
DECLARE
    rid uuid;
    sid uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        rid := OLD.intel_record_id; sid := OLD.intel_source_id;
    ELSE
        rid := NEW.intel_record_id; sid := NEW.intel_source_id;
    END IF;
    IF rid IS NOT NULL THEN
        PERFORM jc_recalc_source_post_count((SELECT source_id FROM intel_records WHERE id = rid));
    END IF;
    IF sid IS NOT NULL THEN
        PERFORM jc_recalc_source_post_count(sid);
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF OLD.intel_record_id IS DISTINCT FROM NEW.intel_record_id AND OLD.intel_record_id IS NOT NULL THEN
            PERFORM jc_recalc_source_post_count((SELECT source_id FROM intel_records WHERE id = OLD.intel_record_id));
        END IF;
        IF OLD.intel_source_id IS DISTINCT FROM NEW.intel_source_id AND OLD.intel_source_id IS NOT NULL THEN
            PERFORM jc_recalc_source_post_count(OLD.intel_source_id);
        END IF;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- video_sources links directly by source_id.
CREATE OR REPLACE FUNCTION trg_video_recalc_source_post_count() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM jc_recalc_source_post_count(OLD.source_id);
        RETURN OLD;
    END IF;
    PERFORM jc_recalc_source_post_count(NEW.source_id);
    IF TG_OP = 'UPDATE' AND OLD.source_id IS DISTINCT FROM NEW.source_id THEN
        PERFORM jc_recalc_source_post_count(OLD.source_id);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_scholarships_post_count
    AFTER INSERT OR DELETE OR UPDATE OF intel_record_id ON scholarships
    FOR EACH ROW EXECUTE FUNCTION trg_projection_recalc_source_post_count();
CREATE TRIGGER trg_intel_jobs_post_count
    AFTER INSERT OR DELETE OR UPDATE OF intel_record_id ON intel_jobs
    FOR EACH ROW EXECUTE FUNCTION trg_projection_recalc_source_post_count();
CREATE TRIGGER trg_remittance_rates_post_count
    AFTER INSERT OR DELETE OR UPDATE OF intel_record_id ON remittance_rates
    FOR EACH ROW EXECUTE FUNCTION trg_projection_recalc_source_post_count();
CREATE TRIGGER trg_wiki_pages_post_count
    AFTER INSERT OR DELETE OR UPDATE OF intel_record_id ON wiki_pages
    FOR EACH ROW EXECUTE FUNCTION trg_projection_recalc_source_post_count();
CREATE TRIGGER trg_contents_post_count
    AFTER INSERT OR DELETE OR UPDATE OF intel_record_id, intel_source_id, deleted_at, status, visibility, content_type
    ON contents FOR EACH ROW EXECUTE FUNCTION trg_contents_recalc_source_post_count();
CREATE TRIGGER trg_video_sources_post_count
    AFTER INSERT OR DELETE OR UPDATE OF source_id, status ON video_sources
    FOR EACH ROW EXECUTE FUNCTION trg_video_recalc_source_post_count();

-- Backfill every source to the renderable count.
SELECT jc_recalc_source_post_count(id) FROM intel_sources;

COMMIT;
