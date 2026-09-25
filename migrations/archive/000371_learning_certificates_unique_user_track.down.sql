-- Reverse BUG-BE-7: drop the (user_id, track_id) uniqueness constraint.
ALTER TABLE learning_certificates
    DROP CONSTRAINT IF EXISTS uq_learning_certificates_user_track;
