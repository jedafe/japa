-- Add UNIQUE(user_id, track_id) to learning_certificates so concurrent
-- learning:issue_certificate retries cannot produce duplicate certificates
-- for the same user/track pair. The existing certificate_number UNIQUE
-- constraint only guards against number collisions, not (user, track) pairs.
-- Ref: §36 learning platform, audit BUG-BE-7.

-- First, remove any pre-existing duplicates (keep the earliest by issued_at).
DELETE FROM learning_certificates a
USING learning_certificates b
WHERE a.user_id = b.user_id
  AND a.track_id = b.track_id
  AND a.id > b.id;

ALTER TABLE learning_certificates
    ADD CONSTRAINT uq_learning_certificates_user_track UNIQUE (user_id, track_id);
