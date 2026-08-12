-- Reverse: drop the removed_by column added in 000416.
ALTER TABLE reviews
    DROP COLUMN removed_by;
