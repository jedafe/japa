-- Seed stock photo RBAC permission nodes.
-- Admin group gets all three; default member roles get search + import only.

INSERT INTO permission_nodes (node, description, category, created_at, updated_at)
VALUES
    ('media.stock.search',   'Browse and search stock photos (Openverse always; others require media.stock.providers)', 'media', now(), now()),
    ('media.stock.import',   'Import stock photos into the media library', 'media', now(), now()),
    ('media.stock.providers', 'Access rate-limited stock providers (Unsplash, Pexels, Pixabay)', 'media', now(), now())
ON CONFLICT (node) DO NOTHING;
