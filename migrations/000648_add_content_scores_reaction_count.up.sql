-- Add reaction_count to content_scores for daily.dev-style feed cards.
-- Backfilled from reactions where entity_type = 'content' so existing posts
-- keep accurate counts when the feed switches to reaction-driven ranking.

ALTER TABLE content_scores
    ADD COLUMN reaction_count int NOT NULL DEFAULT 0;

UPDATE content_scores
SET reaction_count = COALESCE(
    (SELECT count(*) FROM reactions r
     WHERE r.entity_type = 'content'
       AND r.entity_id = content_scores.content_id),
    0
);
