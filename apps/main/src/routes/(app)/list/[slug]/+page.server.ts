import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { contentHref } from '$lib/utils/contentLink';

/**
 * List (listicle) single page at /list/{slug} — the canonical detail route
 * for content_type 'list' (§14 list amendment, ADR-058 route families).
 * Shares the generic content loader; a non-list slug under /list is
 * redirected to its own canonical route.
 *
 * list.enabled is a feature flag (unified rollout) exposed via the root
 * layout's features map — the deprecated public system_settings key is a
 * stale mirror and must not be read here.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (features?.['list.enabled'] !== true) {
		throw error(404, { code: 'feature_disabled', message: 'Lists are not available' });
	}

	const data = await loadContentArticle(event);

	if (data.article.content_type !== 'list') {
		throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
	}

	return data;
};
