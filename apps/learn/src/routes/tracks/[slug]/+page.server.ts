import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull, apiList, ApiError } from '$lib/server/api';
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

	// Public reviews (polymorphic reviews table, entity_type = 'learning_track').
	let reviews: TrackReview[] = [];
	try {
		const res = await apiList<TrackReview>(ctx, `/learn/tracks/${track.id}/reviews?limit=10`);
		reviews = res.items;
	} catch {
		reviews = [];
	}

	// Review aggregate (avg + count).
	let reviewAggregate: ReviewAggregate | null = null;
	try {
		reviewAggregate = await apiGetOrNull<ReviewAggregate>(ctx, `/learn/tracks/${track.id}/reviews/aggregate`);
	} catch {
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
