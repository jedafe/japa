import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { contentHref } from '$lib/utils/contentLink';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Scholarship } from '$lib/types';

/**
 * Guide single page at /guide/{slug} — the canonical detail route for
 * content_type 'guide' (§14, ADR-058 route families). Shares the generic
 * content loader; a non-guide slug under /guide is redirected to its own
 * canonical route.
 *
 * §73 integration: when the guide is also a scholarship (its content id is
 * the scholarship guide_id), load the matching Scholarship so the page can
 * render a "View Scholarship" CTA.
 */
export const load: PageServerLoad = async (event) => {
	const data = await loadContentArticle(event);

	if (data.article.content_type !== 'guide') {
		throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
	}

	const ctx = apiContext(event);
	let scholarship: { guide_id: string; slug: string; title: string } | null = null;
	try {
		const res = await apiGet<Scholarship>(ctx, `/scholarships/${data.article.id}`);
		scholarship = {
			guide_id: res.guide_id,
			slug: res.slug,
			title: res.title
		};
	} catch (err) {
		// 404 simply means this guide is not also in the scholarship finder.
		if (!(err instanceof ApiError && err.status === 404)) {
			// Degrade silently — the guide page should still render.
		}
	}

	return { ...data, scholarship };
};
