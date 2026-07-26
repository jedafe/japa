-- Rollback: Japa University Creator Economy (ADR-063)
-- Drop in reverse dependency order. revenue_splits references both
-- instructor_profiles and instructor_payouts, so it drops first.

ALTER TABLE enrollments
    DROP COLUMN IF EXISTS payment_id;

DROP INDEX IF EXISTS idx_learning_tracks_instructor;
DROP INDEX IF EXISTS idx_learning_tracks_university;
ALTER TABLE learning_tracks
    DROP COLUMN IF EXISTS total_enrolled,
    DROP COLUMN IF EXISTS language,
    DROP COLUMN IF EXISTS category,
    DROP COLUMN IF EXISTS is_published_to_university,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS price_kobo,
    DROP COLUMN IF EXISTS instructor_id;

DROP TABLE IF EXISTS revenue_splits;
DROP TABLE IF EXISTS instructor_payouts;
DROP TABLE IF EXISTS instructor_profiles;
