import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetBody, ApiError } from '$lib/server/api';
import { error } from '@sveltejs/kit';
import type { TrackDetail, TrackReview, ReviewAggregate } from '$lib/types';
import { courseJsonLd, breadcrumbJsonLd } from '$lib/seo';

/**
 * Track Detail — SSR public content only (ADR-060 SSR/CSR boundary).
 * Fetches the track, its curriculum (modules/lessons), and public reviews
 * from the marketplace catalog endpoint.
 * Enrollment state is NEVER fetched here — CSR-only via /me/enrollments.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	const detailPath = `/academy/tracks/${slug}`;

	let track: TrackDetail;
	let backendJsonLd: unknown = null;
	try {
		const body = await apiGetBody<{ data: TrackDetail; json_ld?: unknown }>(ctx, detailPath);
		track = body.data;
		backendJsonLd = body.json_ld ?? null;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'track_not_found', message: 'Track not found' });
		}
		throw err;
	}

	// Public reviews + aggregate. The backend returns both inline from a single
	// GET /academy/tracks/{id}/reviews call (`{ data: [...], aggregate: {...} }`) —
	// there is NO separate /reviews/aggregate route (per api-routes-catalog.md:
	// "GET /academy/tracks/{id}/reviews [public] — list reviews + aggregate").
	// Reading them together avoids a 404 on a non-existent aggregate endpoint.
	let reviews: TrackReview[] = [];
	let reviewAggregate: ReviewAggregate | null = null;
	try {
		const body = await apiGetBody<{
			data?: TrackReview[];
			aggregate?: ReviewAggregate;
		}>(ctx, `/academy/tracks/${track.id}/reviews?page_size=10`);
		reviews = body.data ?? [];
		reviewAggregate = body.aggregate ?? null;
	} catch {
		reviews = [];
		reviewAggregate = null;
	}

	// JSON-LD structured data (Section 14 / Section 26). Prefer the
	// admin-managed Course template resolved server-side (backend `json_ld`
	// sidecar); fall back to the client-side courseJsonLd helper when absent.
	// Breadcrumb is always emitted client-side (no template for it).
	const courseJsonLdNode = backendJsonLd ?? courseJsonLd(track);
	const jsonLd = [
		courseJsonLdNode,
		breadcrumbJsonLd([
			{ name: 'Explore', path: '/' },
			{ name: track.title, path: `/tracks/${slug}` }
		])
	];

	return {
		track,
		reviews,
		reviewAggregate,
		jsonLd
	};
};
