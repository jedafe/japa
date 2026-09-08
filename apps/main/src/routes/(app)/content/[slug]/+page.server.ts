import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { apiContext, apiGet } from '$lib/server/api';
import { contentHref } from '$lib/utils/contentLink';
import type { VideoSource } from '$lib/types';

/**
 * Generic content single page (CPTs without a dedicated route family).
 * ADR-058: types with a canonical route (guide → /guide/{slug},
 * story → /story/{slug}, blog_post → /blog/{slug}, …) are 301-redirected
 * so every content item has exactly one canonical URL.
 *
 * §106 video: when the article is a video post, the linked video_sources
 * row is loaded alongside it so the page can render the player — the
 * stored body only carries an empty .video-embed placeholder.
 */
export const load: PageServerLoad = async (event) => {
	const data = await loadContentArticle(event);

	const canonical = contentHref(data.article);
	if (!canonical.startsWith('/content/')) {
		throw redirect(301, `${canonical}${event.url.search}`);
	}

	// Best-effort video source resolution — a failure degrades to rendering
	// the article without the player rather than failing the whole page.
	let videoSource: VideoSource | null = null;
	if (data.article.content_type === 'video' && data.article.video_source_id) {
		try {
			videoSource = await apiGet<VideoSource>(
				apiContext(event),
				`/videos/${data.article.video_source_id}`
			);
		} catch {
			videoSource = null;
		}
	}

	return { ...data, videoSource };
};
