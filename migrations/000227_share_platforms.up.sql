-- Migration 000227: Share Platforms
--
-- Admin-configurable social media sharing platforms. Each platform has a
-- URL template with {url} and {title} placeholders, an icon identifier,
-- a display label, and an enabled flag + sort order.
--
-- The frontend share dropdown fetches the enabled platforms (public,
-- no auth) and renders share links for each.

CREATE TABLE IF NOT EXISTS share_platforms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,           -- e.g. 'whatsapp', 'telegram', 'facebook', 'twitter'
    label       TEXT NOT NULL,           -- display label e.g. 'WhatsApp'
    icon        TEXT NOT NULL DEFAULT '',-- lucide icon name or emoji
    url_template TEXT NOT NULL,          -- e.g. 'https://wa.me/?text={title}%20{url}'
    color       TEXT NOT NULL DEFAULT '',-- brand color hex e.g. '#25D366'
    enabled     BOOLEAN NOT NULL DEFAULT true,
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, key)
);

CREATE INDEX IF NOT EXISTS idx_share_platforms_tenant ON share_platforms(tenant_id);
CREATE INDEX IF NOT EXISTS idx_share_platforms_tenant_enabled ON share_platforms(tenant_id, enabled);

-- Seed default platforms for the default tenant
INSERT INTO share_platforms (tenant_id, key, label, icon, url_template, color, enabled, position)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'whatsapp', 'WhatsApp', 'MessageCircle', 'https://wa.me/?text={title}%20{url}', '#25D366', true, 0),
    ('00000000-0000-0000-0000-000000000001', 'telegram', 'Telegram', 'Send', 'https://t.me/share/url?url={url}&text={title}', '#0088cc', true, 1),
    ('00000000-0000-0000-0000-000000000001', 'facebook', 'Facebook', 'Facebook', 'https://www.facebook.com/sharer/sharer.php?u={url}', '#1877F2', true, 2),
    ('00000000-0000-0000-0000-000000000001', 'twitter', 'Twitter / X', 'Twitter', 'https://twitter.com/intent/tweet?url={url}&text={title}', '#000000', true, 3),
    ('00000000-0000-0000-0000-000000000001', 'linkedin', 'LinkedIn', 'Linkedin', 'https://www.linkedin.com/sharing/share-offsite/?url={url}', '#0A66C2', true, 4),
    ('00000000-0000-0000-0000-000000000001', 'email', 'Email', 'Mail', 'mailto:?subject={title}&body={url}', '#6B7280', true, 5)
ON CONFLICT (tenant_id, key) DO NOTHING;
