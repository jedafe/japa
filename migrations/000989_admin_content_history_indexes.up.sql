-- Supporting indexes for the §22 moderation-history projection on
-- GET /admin/content/{id} (AdminListContentModerationHistory). Without these,
-- the ai_hitl_reviews and moderation_appeals branches seq-scan on every
-- detail load — the appeals pending-unique index is partial and cannot
-- serve resolved-appeal lookups.
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_content
    ON ai_hitl_reviews (content_id) WHERE content_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_moderation_appeals_entity
    ON moderation_appeals (entity_type, entity_id);
