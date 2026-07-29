import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { UniversityDetail, ProfilePost, SpaceMember } from '$lib/types';

/**
 * University detail at /universities/[slug] — public, SSR, indexable.
 * Universities are spaces (gen.Space rows filtered to the university space
 * type); the /universities/{slug} endpoint returns the Space shape plus
 * resolved custom field values and a logo_url (imgproxy-signed from
 * logo_media_id). Renders the university header, stats, and tabbed content
 * (Overview, Posts, Forums, Courses, Members, Reviews, Compare) per
 * Section 39 / ADR-039, plus a CTA to the space discussions hub and a
 * "Compare" button linking to /compare?add={slug}.
 *
 * Ref: japaconnect-engineering/sections/39-universities.md
 *      japaconnect-engineering/sections/22-spaces.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let university: UniversityDetail;
	try {
		university = await apiGet<UniversityDetail>(ctx, `/universities/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'University not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load university' });
	}

	// Best-effort fetch of all tab content in parallel. Each degrades to [] on
	// failure so a single endpoint outage never blanks the whole page.
	const [posts, reviews, members, forums] = await Promise.all([
		apiList<ProfilePost>(ctx, `/spaces/${university.id}/content?page_size=10`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<unknown>(ctx, `/spaces/${university.id}/reviews?page_size=10`)
			.then((r) => r.items as Array<{ id: string; rating: number; title?: string; body?: string; author_name?: string; created_at: string }>)
			.catch(() => []),
		apiList<SpaceMember>(ctx, `/spaces/${university.id}/members?page_size=12`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<ProfilePost>(ctx, `/spaces/${university.id}/content?page_size=10&content_type=forum_thread`)
			.then((r) => r.items)
			.catch(() => [])
	]);

	return { university, posts, reviews, members, forums };
};
