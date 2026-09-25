-- Supports the For-You candidate window (recent published/public content).
CREATE INDEX IF NOT EXISTS idx_contents_foryou_window
    ON contents (tenant_id, status, visibility, created_at DESC);

-- Supports the velocity candidate ordering.
CREATE INDEX IF NOT EXISTS idx_content_scores_velocity
    ON content_scores (velocity_score DESC);

-- Supports member-space affinity lookup by user.
CREATE INDEX IF NOT EXISTS idx_space_members_user
    ON space_members (tenant_id, user_id);

-- Supports tag-overlap EXISTS/JOIN by tag.
CREATE INDEX IF NOT EXISTS idx_content_tags_tag
    ON content_tags (tag_id);
