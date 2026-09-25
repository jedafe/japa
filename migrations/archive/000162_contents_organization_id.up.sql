-- Post-as-organization (ADR-062): dual attribution. author_id stays NOT NULL
-- (the human author, for moderation/accountability); organization_id is the
-- optional org the post speaks for.
--
-- ON DELETE SET NULL (matches the media.user_group_id nullable-attribution
-- precedent): if an org is ever hard-deleted, its posts survive as their human
-- author's content and simply lose the org attribution — never cascade-delete
-- human-authored content.
ALTER TABLE contents ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL;

-- Powers the org Posts tab and org-attributed lookups: org's published-public
-- posts, newest first. Partial (only org-attributed, live rows) to stay small.
CREATE INDEX idx_contents_org ON contents (tenant_id, organization_id, status, published_at DESC)
  WHERE organization_id IS NOT NULL AND deleted_at IS NULL;
