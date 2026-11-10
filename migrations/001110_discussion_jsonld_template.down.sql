-- 001110_discussion_jsonld_template.down.sql
-- Removes the seeded DiscussionForumPosting template only when it still
-- carries the seed body — an admin-edited template survives the rollback.

BEGIN;

DELETE FROM content_type_jsonld_templates
WHERE content_type = 'DiscussionForumPosting'
  AND template = '{"@type": "DiscussionForumPosting", "@context": "https://schema.org", "headline": "{{title}}", "text": "{{body}}", "url": "{{canonical_url}}", "image": "{{cover_image_url}}", "author": {"url": "{{author_profile_url}}", "name": "{{author_name}}", "@type": "Person"}, "datePublished": "{{published_at}}", "dateModified": "{{updated_at}}", "commentCount": "{{comment_count}}", "publisher": {"logo": {"url": "https://japa.to/logo.png", "@type": "ImageObject"}, "name": "JapaConnect", "@type": "Organization"}}'::jsonb;

COMMIT;
