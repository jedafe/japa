// Shared bookmark entity→URL/label mapping for the profile Saved surfaces —
// resolves each entity to the bookmarked item's canonical page (cross-app
// deep links per ADR-060).
import { t } from '$lib/i18n';
import { academyUrl, docsUrl } from '$lib/appOrigins';
import type { Bookmark } from '$lib/types';

function safeAcademyUrl(path: string): string {
	try {
		return academyUrl(path);
	} catch {
		return '#';
	}
}
function safeDocsUrl(path: string): string {
	try {
		return docsUrl(path);
	} catch {
		return '#';
	}
}

export function bookmarkUrl(b: Bookmark): string {
	// Prefer the slug for content/blog posts (canonical, SEO-friendly URL).
	const idOrSlug = b.slug || b.entity_id;
	switch (b.entity_type) {
		// /content/{slug} is the type-aware resolver — it 301s to the right
		// surface (/blog, /services, …). Linking 'content' to /blog/ 404s
		// whenever the bookmarked content is not a blog post.
		case 'content':
			return `/content/${idOrSlug}`;
		case 'blog_post':
			return `/blog/${idOrSlug}`;
		case 'forum_thread':
			return `/forum/${idOrSlug}`;
		case 'forum_reply':
			return `/forum/${idOrSlug}#reply-${b.entity_id}`;
		case 'space':
			return `/spaces/${idOrSlug}`;
		case 'tag':
			return `/tags/${idOrSlug}`;
		case 'comment':
			return `/content/${b.entity_id}`;
		// Doc pages live on the Docs app under /docs/{audience}/{slug}.
		case 'doc_page':
			return safeDocsUrl(`/docs/${b.audience ?? 'users'}/${idOrSlug}`);
		// Academy tracks live on the Academy app.
		case 'academy_track':
			return safeAcademyUrl(`/tracks/${idOrSlug}`);
		default:
			return `/content/${b.entity_id}`;
	}
}

export function bookmarkLabel(b: Bookmark): string {
	if (b.title) return b.title;
	const key = `bookmarks.type.${b.entity_type}`;
	return `${t(key, b.entity_type)} #${b.entity_id.slice(0, 8)}`;
}
