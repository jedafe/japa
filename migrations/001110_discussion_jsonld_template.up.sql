-- 001110_discussion_jsonld_template.up.sql
-- Seeds the DiscussionForumPosting JSON-LD template for discussion and
-- forum_thread content served via the content API (the forum handler's
-- QAPage path is unchanged). Discussion pages carry unique user-generated
-- comments, so Article markup underreports them — DiscussionForumPosting is
-- the schema.org type Google uses for forum/discussion rich results.

BEGIN;

INSERT INTO content_type_jsonld_templates (content_type, template, is_active)
VALUES (
	'DiscussionForumPosting',
	'{"@type": "DiscussionForumPosting", "@context": "https://schema.org", "headline": "{{title}}", "text": "{{description}}", "url": "{{canonical_url}}", "image": "{{cover_image_url}}", "author": {"url": "{{author_profile_url}}", "name": "{{author_name}}", "@type": "Person"}, "datePublished": "{{published_at}}", "dateModified": "{{updated_at}}", "commentCount": "{{comment_count}}", "publisher": {"logo": {"url": "https://japa.to/logo.png", "@type": "ImageObject"}, "name": "JapaConnect", "@type": "Organization"}}'::jsonb,
	true
)
ON CONFLICT (content_type) DO NOTHING;

COMMIT;
