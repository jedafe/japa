-- 000520_studio_jobs_approach_b_fk_fix.down.sql
-- Re-adds the FKs removed by 000520. This is only safe if 000520 has not yet
-- stored any tool-table IDs in draft_content_id or ai_hitl_reviews IDs in
-- review_id. If such rows exist, clean them first (set those columns to NULL
-- or delete the rows) before running this down migration.

BEGIN;

ALTER TABLE studio_jobs
    ADD CONSTRAINT studio_jobs_draft_content_id_fkey
        FOREIGN KEY (draft_content_id) REFERENCES contents(id),
    ADD CONSTRAINT studio_jobs_review_id_fkey
        FOREIGN KEY (review_id) REFERENCES content_reviews(id);

COMMIT;
