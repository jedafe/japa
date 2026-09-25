-- Section 29: Add removed_by column to reviews table.
-- Spec §29 L55: `removed_by uuid REFERENCES users(id) ON DELETE SET NULL`
-- Tracks which user removed a review (the reviewer for self-delete, or the
-- moderator for admin removal). NULL for published/flagged reviews.
ALTER TABLE reviews
    ADD COLUMN removed_by uuid REFERENCES users(id) ON DELETE SET NULL;
