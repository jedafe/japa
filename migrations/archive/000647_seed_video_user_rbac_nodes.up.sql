-- §106 / ADR-117: user-level video permission nodes.
BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('video.create', 'Create a video post from /create'),
    ('video.import.single', 'Import or resolve a single public video URL'),
    ('video.import.bulk', 'Start and retry bulk YouTube channel/playlist/keyword imports'),
    ('video.channel.claim', 'Submit a channel ownership claim or takedown request')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'admin', 'member')
  AND p.key IN (
      'video.create',
      'video.import.single',
      'video.import.bulk',
      'video.channel.claim'
  )
ON CONFLICT DO NOTHING;

COMMIT;
