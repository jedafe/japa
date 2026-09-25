ALTER TABLE space_pinned_posts ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE;

UPDATE space_pinned_posts SET tenant_id = s.tenant_id
FROM spaces s
WHERE space_pinned_posts.space_id = s.id
  AND space_pinned_posts.tenant_id IS NULL;

ALTER TABLE space_pinned_posts ALTER COLUMN tenant_id SET NOT NULL;
