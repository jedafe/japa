-- Add flagged column to media table.
-- "flagged" is distinct from "quarantined": a media item can be flagged by
-- spam/AI moderation or user reports while remaining visible pending review.
-- Quarantined media is hidden; flagged media is visible but marked for review.
ALTER TABLE media ADD COLUMN IF NOT EXISTS flagged boolean NOT NULL DEFAULT false;
