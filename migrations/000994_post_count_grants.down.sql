-- 000994_post_count_grants.down.sql — reverts to the 000993 recount
-- (contents-linked types without 'grants').

BEGIN;

CREATE OR REPLACE FUNCTION jc_recalc_source_post_count(p_source_id uuid) RETURNS void AS $$
BEGIN
    UPDATE intel_sources SET post_count = (
        (SELECT COUNT(*) FROM intel_records ir
          WHERE ir.source_id = p_source_id AND ir.status = 'published' AND (
               (ir.entity_type = 'scholarship' AND EXISTS (SELECT 1 FROM scholarships s WHERE s.intel_record_id = ir.id))
            OR (ir.entity_type = 'job' AND EXISTS (SELECT 1 FROM intel_jobs j WHERE j.intel_record_id = ir.id))
            OR (ir.entity_type = 'remittance' AND EXISTS (SELECT 1 FROM remittance_rates rr WHERE rr.intel_record_id = ir.id))
            OR (ir.entity_type IN ('news','culture_lifestyle','guide','blog','article')
                AND EXISTS (SELECT 1 FROM contents bc WHERE bc.intel_record_id = ir.id AND bc.deleted_at IS NULL))
            OR (ir.entity_type = 'wiki_page' AND EXISTS (SELECT 1 FROM wiki_pages w WHERE w.intel_record_id = ir.id))
          ))
        + (SELECT COUNT(*) FROM video_sources vs WHERE vs.source_id = p_source_id AND vs.status = 'active')
        + (SELECT COUNT(*) FROM contents c
            WHERE c.intel_source_id = p_source_id
              AND c.content_type = 'comparison'
              AND c.deleted_at IS NULL
              AND c.status = 'published'
              AND c.visibility = 'public')
    ) WHERE id = p_source_id;
END;
$$ LANGUAGE plpgsql;

SELECT jc_recalc_source_post_count(id) FROM intel_sources;

COMMIT;
