-- Wiki Product Domain (§80 Product Domain amendment, spec
-- 2026-11-06-wiki-dynamic-entity-kinds.md):
-- product_types (kind lookup — new kind = insert, not schema),
-- products (typed core + cfv long tail),
-- product_providers (N:M resellers — kilo/devin/opencode offering the
-- same product at different prices).
-- Providers resolve to the existing affiliate_providers registry;
-- "deals" = coupons rows on those providers.
BEGIN;

CREATE TABLE product_types (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    label       text NOT NULL,
    icon        text,
    is_enabled  boolean NOT NULL DEFAULT true,
    config      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_type_id uuid NOT NULL REFERENCES product_types(id) ON DELETE RESTRICT,
    slug            text NOT NULL UNIQUE,
    name            text NOT NULL,
    tagline         text,
    website_url     text,
    logo_id         uuid REFERENCES media(id) ON DELETE SET NULL,
    pricing_model   text,
    price_from      numeric,
    rating_avg      numeric,
    review_count    integer NOT NULL DEFAULT 0,
    geo_space_id    uuid REFERENCES spaces(id) ON DELETE SET NULL,
    created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    deleted_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_products_type ON products (product_type_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_slug ON products (slug) WHERE deleted_at IS NULL;

CREATE TABLE product_providers (
    product_id   uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    provider_id  uuid NOT NULL REFERENCES affiliate_providers(id) ON DELETE CASCADE,
    price_from   numeric,
    offer_url    text,
    position     integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, provider_id)
);
CREATE INDEX idx_product_providers_provider ON product_providers (provider_id);

-- Seed product types (config insert per the Product model — new kinds
-- like 'hosting', 'cloud' are added the same way via the admin API).
INSERT INTO product_types (slug, label, icon) VALUES
    ('ai-agent',    'AI Agent',    'sparkles'),
    ('ai-model',    'AI Model',    'cpu'),
    ('web-app',     'Web App',     'globe'),
    ('website',     'Website',     'layout'),
    ('saas',        'SaaS',        'cloud'),
    ('mobile-app',  'Mobile App',  'smartphone'),
    ('hosting',     'Hosting',     'server'),
    ('cloud',       'Cloud',       'cloud')
ON CONFLICT (slug) DO NOTHING;

-- 'reviews' content type — §14 machinery (slugs, moderation, comments,
-- feeds, content_hash) with product_id + rating + providers repeater in
-- contents.config.
INSERT INTO permissions (key, description) VALUES
    ('review.create', 'Create product review content via the composer or POST /content'),
    ('agent.content_type.review', 'AI Agent — create review content')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key = 'review.create'
ON CONFLICT DO NOTHING;

-- agent.content_type.review inherits the agent.content_type family grants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT rp.role_id, p.id
FROM role_permissions rp
JOIN permissions src ON src.id = rp.permission_id AND src.key = 'agent.content_type.blog_post'
JOIN permissions p ON p.key = 'agent.content_type.review'
ON CONFLICT DO NOTHING;

-- Review JSON-LD template — reviewBody/author injected at render time.
INSERT INTO content_type_jsonld_templates (content_type, template, is_active) VALUES (
  'review',
  '{"@context":"https://schema.org","@type":"Review","headline":"{{title}}","reviewBody":"{{description}}","author":{"@type":"Person","name":"{{author}}"},"datePublished":"{{published_at}}","dateModified":"{{updated_at}}","image":"{{image_url}}"}',
  true)
ON CONFLICT (content_type) DO NOTHING;

COMMIT;
