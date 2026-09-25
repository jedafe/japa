-- 000352_seed_seo_sitemap_quality_gates.up.sql
-- Seeds the Section 43 sitemap quality gate thresholds into system_settings
-- so they appear in the admin panel Settings -> SEO category and are
-- configurable per tenant.
--
-- Keys:
--   Space-level gates:
--     seo.sitemap.space_min_members   — min member count for a space
--     seo.sitemap.space_min_posts     — min total post count for a space
--     seo.sitemap.space_min_forums    — min forum thread count for a space
--     seo.sitemap.space_min_questions — min solved question count for a space
--
--   Content-level word count gates (per type + fallback):
--     seo.sitemap.content_min_words     — fallback (default 100)
--     seo.sitemap.blog_min_words        — blog posts (default 100)
--     seo.sitemap.forum_min_words       — forum threads (default 30)
--     seo.sitemap.discussion_min_words  — discussions (default 50)
--     seo.sitemap.guide_min_words       — guides (default 200)
--     seo.sitemap.comparison_min_words  — comparisons (default 50)
--
--   Content-level engagement gates (per type):
--     seo.sitemap.blog_min_engagement     — min comments+shares+reactions (default 0)
--     seo.sitemap.forum_min_replies       — min reply count (default 1)
--     seo.sitemap.question_min_answers    — min answers for questions (default 1, 0=include unsolved)
--     seo.sitemap.discussion_min_comments — min comment count (default 0)
--
-- Ref: japaconnect-engineering/sections/43-bot-crawler-management.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- Space-level gates
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.space_min_members', '1'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.space_min_posts', '1'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.space_min_forums', '1'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.space_min_questions', '1'),
    -- Content-level word count gates
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.content_min_words', '100'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.blog_min_words', '100'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.forum_min_words', '30'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.discussion_min_words', '50'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.guide_min_words', '200'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.comparison_min_words', '50'),
    -- Content-level engagement gates
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.blog_min_engagement', '0'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.forum_min_replies', '1'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.question_min_answers', '1'),
    ('00000000-0000-0000-0000-000000000001', 'seo.sitemap.discussion_min_comments', '0')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
