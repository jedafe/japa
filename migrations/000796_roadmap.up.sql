-- 000796_roadmap.up.sql
--
-- Public product roadmap (§113): community-votable, admin-curated feature
-- roadmap at /roadmap and roadmap.japa.to. Items carry a hidden
-- contents row (content_type='roadmap') so the §49 comments engine serves
-- item discussion threads unchanged. vote_count is a denormalized counter
-- kept honest by a recount query (ADR-127), not increment/decrement drift.
--
-- Seeds: five default categories, the roadmap.* permission nodes (vote +
-- submit to member tiers, all nodes to admin roles), the roadmap_shipped
-- notification type consumed by the roadmap:ship_notify task, and the
-- flagship 'spaces-content-creation' item (in_progress).
--
-- Ref: .jeos/sections/113-public-product-roadmap.md
--      .jeos/adr/ADR-127-public-product-roadmap.md

BEGIN;

CREATE TABLE roadmap_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    description text,
    color       text,
    sort_order  int  NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TABLE roadmap_items (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title            text NOT NULL,
    slug             text NOT NULL UNIQUE,
    summary          text,
    body             text,
    category_id      uuid REFERENCES roadmap_categories(id) ON DELETE SET NULL,
    status           text NOT NULL DEFAULT 'under_review'
                     CHECK (status IN ('pending','under_review','planned',
                                       'in_progress','shipped','declined','archived')),
    sort_order       int  NOT NULL DEFAULT 0,
    is_flagship      bool NOT NULL DEFAULT false,
    target_window    text,
    feature_flag_key text,
    content_id       uuid REFERENCES contents(id) ON DELETE SET NULL,
    vote_count       int  NOT NULL DEFAULT 0,
    submitted_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    approved_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    shipped_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);

CREATE INDEX idx_roadmap_items_status   ON roadmap_items (status, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_roadmap_items_category ON roadmap_items (category_id)        WHERE deleted_at IS NULL;
CREATE INDEX idx_roadmap_items_shipped  ON roadmap_items (shipped_at DESC)    WHERE deleted_at IS NULL AND status = 'shipped';

CREATE TABLE roadmap_votes (
    item_id    uuid NOT NULL REFERENCES roadmap_items(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, user_id)
);

INSERT INTO roadmap_categories (slug, name, description, color, sort_order) VALUES
    ('spaces-content', 'Spaces & Content', 'Spaces, content creation, and publishing',                '#6366f1', 0),
    ('community',      'Community',        'Forums, groups, and social features',                     '#22c55e', 1),
    ('tools',          'Tools',            'Calculators, checklists, and guides',                     '#f59e0b', 2),
    ('mobile',         'Mobile',           'iOS and Android apps',                                    '#06b6d4', 3),
    ('platform',       'Platform',         'API, infrastructure, and developer experience',           '#a855f7', 4)
ON CONFLICT (slug) DO NOTHING;

-- roadmap.* permission nodes (§113, conventions/permission-key-naming.md).
INSERT INTO permissions (key, description) VALUES
    ('roadmap.vote',     'Vote on public roadmap items'),
    ('roadmap.submit',   'Submit feature requests to the public roadmap'),
    ('roadmap.view',     'View roadmap items and categories in the admin panel'),
    ('roadmap.create',   'Create roadmap items'),
    ('roadmap.edit',     'Edit roadmap items and categories, restore, reorder'),
    ('roadmap.delete',   'Delete roadmap items'),
    ('roadmap.moderate', 'Triage submissions, change statuses, and inline-edit the public roadmap')
ON CONFLICT (key) DO NOTHING;

-- Member tiers may vote and submit.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key IN ('roadmap.vote', 'roadmap.submit')
ON CONFLICT DO NOTHING;

-- Admin roles get every roadmap node.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key LIKE 'roadmap.%'
ON CONFLICT DO NOTHING;

-- Notification type for roadmap:ship_notify (§113). Notification lists JOIN
-- notification_types on key — an unseeded key silently drops the rows.
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('roadmap_shipped', 'Roadmap item shipped',
     'A roadmap item you voted for has shipped',
     true, false, false, 'system')
ON CONFLICT (key) DO NOTHING;

-- Flagship: the first wave — Spaces & Content Creation (in_progress).
INSERT INTO roadmap_items (title, slug, summary, body, category_id, status, sort_order, is_flagship, target_window)
SELECT 'Spaces & Content Creation', 'spaces-content-creation',
       'The core of JapaConnect: geo-first spaces and rich content creation — posts, articles, media, and moderation tools.',
       'The core of JapaConnect: geo-first spaces (Country → State → City → University) and rich content creation — posts, articles, media, drafts, and moderation tools. This is the first feature wave going live.',
       c.id, 'in_progress', 0, true, 'Q4 2026'
FROM roadmap_categories c
WHERE c.slug = 'spaces-content'
ON CONFLICT (slug) DO NOTHING;

COMMIT;
