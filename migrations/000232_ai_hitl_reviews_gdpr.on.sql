-- 000232_ai_hitl_reviews_gdpr.on.sql
-- Section 19 (GDPR/NDPR) + Section 20 (AI Core) conformance fix.
--
-- BUG-012: ai_hitl_reviews.reviewer_id REFERENCES users(id) with no ON DELETE
-- clause, so the GDPR account:deletion_cleanup hard-delete of a user would FK-
-- violate (or, if it skipped the row, leave reviewer PII behind). Switch the FK
-- to ON DELETE SET NULL so a deleted user's HITL reviews are retained for audit
-- but the reviewer identity is nullified (anonymized) — matching the
-- soft-delete-and-anonymize principle (Rule 11, §15).
--
-- The existing reviewer_id column is already nullable, so SET NULL is safe.

ALTER TABLE ai_hitl_reviews
    DROP CONSTRAINT IF EXISTS ai_hitl_reviews_reviewer_id_fkey,
    ADD CONSTRAINT ai_hitl_reviews_reviewer_id_fkey
        FOREIGN KEY (reviewer_id) REFERENCES users(id) ON DELETE SET NULL;
