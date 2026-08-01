import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetBody, ApiError } from '$lib/server/api';
import { error } from '@sveltejs/kit';
import type { TrackDetail, TrackReview, ReviewAggregate } from '$lib/types';
import { courseJsonLd, breadcrumbJsonLd } from '$lib/seo';

/**
 * Track Detail — SSR public content only (ADR-060 SSR/CSR boundary).
 * Fetches the track, its curriculum (modules/lessons), and public reviews.
 * Enrollment state is NEVER fetched here — CSR-only via /me/enrollments.
 *
 * On the university host, fetches the marketplace catalog detail instead.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const isUniversity = event.locals.isUniversityHost;

	const detailPath = isUniversity
		? `/learn/university/catalog/${slug}`
		: `/learn/tracks/${slug}`;

	let track: TrackDetail;
	try {
		track = await apiGet<TrackDetail>(ctx, detailPath);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'track_not_found', message: 'Track not found' });
		}
		throw err;
	}

	// Public reviews + aggregate. The backend returns both inline from a single
	// GET /learn/tracks/{id}/reviews call (`{ data: [...], aggregate: {...} }`) —
	// there is NO separate /reviews/aggregate route (per api-routes-catalog.md:
	// "GET /learn/tracks/{id}/reviews [public] — list reviews + aggregate").
	// Reading them together avoids a 404 on a non-existent aggregate endpoint.
	let reviews: TrackReview[] = [];
	let reviewAggregate: ReviewAggregate | null = null;
	try {
		const body = await apiGetBody<{
			data?: TrackReview[];
			aggregate?: ReviewAggregate;
		}>(ctx, `/learn/tracks/${track.id}/reviews?limit=10`);
		reviews = body.data ?? [];
		reviewAggregate = body.aggregate ?? null;
	} catch {
		reviews = [];
		reviewAggregate = null;
	}

	// JSON-LD structured data (Course schema + Breadcrumb).
	const jsonLd = [
		courseJsonLd(track),
		breadcrumbJsonLd([
			{ name: 'Explore', path: '/' },
			{ name: track.title, path: `/tracks/${slug}` }
		])
	];

	return {
		track,
		reviews,
		reviewAggregate,
		isUniversity,
		jsonLd
	};
};
