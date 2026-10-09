// Ref: japaconnect-engineering/sections/114-services-marketplace.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope, apiList, ApiError } from '$lib/server/api';
import type { ServiceListing, ServiceReview, ServiceReviewAggregate } from '$lib/types';

// Matches the backend's default ?limit= on GET /services/{id}/reviews
// (parseReviewPageParams — limit 10, max 50).
const PAGE_SIZE = 10;

/**
 * Service reviews index at /services/[slug]/reviews — paginated SSR list
 * backed by GET /services/{id}/reviews. The route is slug-keyed but the
 * API takes the listing's contents id, so the load first resolves the
 * listing (same apiGetEnvelope lookup as the detail page — the envelope
 * also carries the review_aggregate sidecar for the header). A 404 on
 * either lookup throws a SvelteKit 404; other review-fetch failures
 * degrade to loadError: true so the page can render a fallback.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_directory.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service directory is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;
	const rawPage = Number(event.url.searchParams.get('page'));
	const pageNum = Number.isInteger(rawPage) && rawPage > 0 ? rawPage : 1;

	let service: ServiceListing;
	let reviewAggregate: ServiceReviewAggregate;
	try {
		const envelope = await apiGetEnvelope<ServiceListing>(ctx, `/services/${slug}`);
		service = envelope.data;
		reviewAggregate =
			(envelope as unknown as { review_aggregate?: ServiceReviewAggregate }).review_aggregate ??
			{ count: 0, avg: 0 };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load service' });
	}

	try {
		const { items, meta } = await apiList<ServiceReview>(
			ctx,
			`/services/${service.id}/reviews?page=${pageNum}&limit=${PAGE_SIZE}`
		);
		return {
			service,
			reviewAggregate,
			reviews: items,
			// The endpoint always emits meta{page,limit,total}; the fallback
			// keeps PaginationControls sane if the reviews fetcher unwires.
			meta: meta ?? { page: pageNum, limit: PAGE_SIZE, total: reviewAggregate.count },
			currentPage: pageNum,
			loadError: false
		};
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		return {
			service,
			reviewAggregate,
			reviews: [] as ServiceReview[],
			meta: undefined,
			currentPage: pageNum,
			loadError: true
		};
	}
};
