-- 000232_ai_hitl_reviews_gdpr.down.sql
-- Reverses 000232: restores the original FK with no ON DELETE clause.

ALTER TABLE ai_hitl_reviews
    DROP CONSTRAINT IF EXISTS ai_hitl_reviews_reviewer_id_fkey,
    ADD CONSTRAINT ai_hitl_reviews_reviewer_id_fkey
        FOREIGN KEY (reviewer_id) REFERENCES users(id);
