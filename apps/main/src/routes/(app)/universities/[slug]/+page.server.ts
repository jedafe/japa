import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError, API_V1 } from '$lib/server/api';
import type { UniversityDetail, ProfilePost, SpaceMember, Review, ReviewAggregate, CatalogTrack } from '$lib/types';

/**
 * University detail at /universities/[slug] — public, SSR, indexable.
 * Universities are spaces (gen.Space rows filtered to the university space
 * type); the /universities/{slug} endpoint returns the Space shape plus
 * resolved custom field values and a logo_url (imgproxy-signed from
 * logo_media_id). Renders the university header, stats, and tabbed content
 * (Overview, Posts, Forums, Courses, Members, Reviews, Compare) per
 * Section 39 / ADR-039, plus a CTA to the space discussions hub and a
 * "Compare" button linking to the JCE picker with this university pre-selected.
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

	// §39 — Resolve the comparison picker URL from the backend's redirect
	// endpoint (GET /universities/{slug}/compare) rather than hardcoding the
	// /compare/universities path and query shape in the frontend.
	let compareHref = `/compare?type=universities&source=university&preselect=${encodeURIComponent(slug)}`;
	try {
		const res = await ctx.fetch(`${API_V1}/universities/${slug}/compare`, {
			method: 'GET',
			headers: {
				host: ctx.host,
				'x-forwarded-host': ctx.host
			},
			redirect: 'manual'
		});
		const location = res.headers.get('location');
		if (location && res.status >= 300 && res.status < 400) {
			// The backend returns a relative path; strip any origin prefix if
			// it is ever configured to return an absolute URL.
			compareHref = location.replace(/^https?:\/\/[^/]+/, '');
		}
	} catch {
		// Keep the fallback path if the redirect endpoint is unavailable.
	}

	// Best-effort fetch of all tab content in parallel. Each degrades to [] on
	// failure so a single endpoint outage never blanks the whole page.
	// Reviews use the canonical catalog routes (Section 29):
	// GET /reviews/{entity_type}/{entity_id} and .../aggregate.
	const [posts, reviews, reviewAggregate, members, forums, courses] = await Promise.all([
		apiList<ProfilePost>(ctx, `/spaces/${university.id}/content?page_size=10`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<Review>(ctx, `/reviews/university/${university.id}?page_size=10`)
			.then((r) => r.items)
			.catch(() => []),
		apiGet<ReviewAggregate>(ctx, `/reviews/university/${university.id}/aggregate`)
			.then((r) => r)
			.catch(() => null),
		apiList<SpaceMember>(ctx, `/spaces/${university.id}/members?page_size=12`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<ProfilePost>(ctx, `/spaces/${university.id}/content?page_size=10&content_type=forum_thread`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<CatalogTrack>(ctx, `/learn/university/catalog?space_id=${university.id}&limit=10`)
			.then((r) => r.items)
			.catch(() => [])
	]);

	return { university, posts, reviews, reviewAggregate, members, forums, courses, compareHref };
};
