-- 000656_sources_post_count_trigger.up.sql
-- Maintain intel_sources.post_count automatically as intel_records move in and
-- out of 'published' status. A single trigger covers all intelligence domains
-- so the count stays consistent without requiring every promoter to hand-roll
-- an increment/decrement call.

BEGIN;

CREATE OR REPLACE FUNCTION trg_intel_records_post_count_sync()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status = 'published' THEN
            UPDATE intel_sources
            SET post_count = post_count + 1
            WHERE id = NEW.source_id;
        END IF;
        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            IF NEW.status = 'published' AND OLD.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources
                SET post_count = post_count + 1
                WHERE id = NEW.source_id;
            ELSIF OLD.status = 'published' AND NEW.status IS DISTINCT FROM 'published' THEN
                UPDATE intel_sources
                SET post_count = GREATEST(0, post_count - 1)
                WHERE id = NEW.source_id;
            END IF;
        END IF;
        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.status = 'published' THEN
            UPDATE intel_sources
            SET post_count = GREATEST(0, post_count - 1)
            WHERE id = OLD.source_id;
        END IF;
        RETURN OLD;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_intel_records_post_count ON intel_records;
CREATE TRIGGER trg_intel_records_post_count
    AFTER INSERT OR UPDATE OF status OR DELETE ON intel_records
    FOR EACH ROW
    EXECUTE FUNCTION trg_intel_records_post_count_sync();

-- Backfill post_count from current published records.
UPDATE intel_sources
SET post_count = (
    SELECT COUNT(*)
    FROM intel_records
    WHERE intel_records.source_id = intel_sources.id
      AND intel_records.status = 'published'
);

COMMIT;
