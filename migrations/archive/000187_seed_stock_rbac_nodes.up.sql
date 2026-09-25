-- Seed stock photo RBAC permission nodes.
-- Admin group gets all three; default member roles get search + import only.

INSERT INTO permissions (key, description) VALUES
    ('media.stock.search', 'Browse and search stock photos (Openverse always; others require media.stock.providers)'),
    ('media.stock.import', 'Import stock photos into the media library'),
    ('media.stock.providers', 'Access rate-limited stock providers (Unsplash, Pexels, Pixabay)')
ON CONFLICT (key) DO NOTHING;
