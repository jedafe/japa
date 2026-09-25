-- 000352_seed_seo_sitemap_quality_gates.down.sql
-- Removes all Section 43 sitemap quality gate thresholds from system_settings.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'seo.sitemap.space_min_members',
    'seo.sitemap.space_min_posts',
    'seo.sitemap.space_min_forums',
    'seo.sitemap.space_min_questions',
    'seo.sitemap.content_min_words',
    'seo.sitemap.blog_min_words',
    'seo.sitemap.forum_min_words',
    'seo.sitemap.discussion_min_words',
    'seo.sitemap.guide_min_words',
    'seo.sitemap.comparison_min_words',
    'seo.sitemap.blog_min_engagement',
    'seo.sitemap.forum_min_replies',
    'seo.sitemap.question_min_answers',
    'seo.sitemap.discussion_min_comments'
  );

COMMIT;
