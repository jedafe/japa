-- 001091 — thread_anchors: binds a lazily-created `discussion` contents row
-- to a non-content entity (tool page, wiki page) so the §49 comment engine
-- can anchor a thread to it. Powers /thread/{slug} entity threads and the
-- discussions block on /tools/{tool} and /wiki/{space}/{page}.
-- Spec: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md

BEGIN;

-- Platform-run space that owns every anchor contents row. Wiki spaces can't
-- be reused (wiki-only space types reject comments) and tool pages have no
-- space — anchor threads file here so comments stay open to members.
-- space_type_id '...63' is the seeded 'community' space type (baseline seed);
-- creator_user_id NULL marks the space as platform-run (is_primary).
INSERT INTO public.spaces (id, space_type_id, name, slug, description, level, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
VALUES ('9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01', '00000000-0000-0000-0000-000000000063', 'Community Threads', 'community-threads', 'Threads attached to tools and wiki pages.', 'community', 'public', 'members', 'members', 'members', 'open', 'active')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE thread_anchors (
    id uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    -- The backing contents row (content_type='discussion') that carries the
    -- comment thread and the /thread/{slug} slug.
    content_id uuid NOT NULL UNIQUE REFERENCES contents(id) ON DELETE CASCADE,
    -- 'tool' | 'wiki_page' (extensible to other non-content entities).
    entity_type text NOT NULL,
    -- tool slug (e.g. 'visa-checklist') | wiki_pages.id (uuid text).
    entity_key text NOT NULL,
    -- Canonical frontend path of the entity for the embed CTA.
    entity_url text NOT NULL,
    entity_title text NOT NULL,
    entity_summary text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT thread_anchors_entity_key UNIQUE (entity_type, entity_key),
    CONSTRAINT thread_anchors_entity_type_check CHECK (entity_type IN ('tool', 'wiki_page'))
);

CREATE INDEX idx_thread_anchors_entity ON thread_anchors (entity_type, entity_key);

COMMIT;
