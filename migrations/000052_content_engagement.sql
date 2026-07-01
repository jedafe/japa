-- +migrate Up
-- Content engagement tracking for velocity scoring (Section 11).
-- Separate from content_scores which is used by the feed ranking pipeline.
CREATE TABLE IF NOT EXISTS content_engagement (
    content_id      UUID PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
    views           INT NOT NULL DEFAULT 0,
    comments_count  INT NOT NULL DEFAULT 0,
    votes_score     INT NOT NULL DEFAULT 0,
    bookmarks_count INT NOT NULL DEFAULT 0,
    shares_count    INT NOT NULL DEFAULT 0,
    velocity_score  FLOAT NOT NULL DEFAULT 0,
    last_calculated_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_content_engagement_velocity
    ON content_engagement(velocity_score DESC);

-- +migrate Down
DROP TABLE IF EXISTS content_engagement;
