// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList, ApiError } from '$lib/server/api';
import type {
	ServiceListing,
	ServiceReview,
	ServiceReviewAggregate,
	ServiceCategoryInfo
} from '$lib/types';
import type { ContentArticle } from '$lib/types/content';

/**
 * Service detail at /services/[slug] — SSR detail. Fetches
 * GET /services/{slug} which returns the listing plus reviews (§29)
 * and a review_aggregate sidecar (BUG-014). On 404, throws a SvelteKit
 * 404; other errors degrade to loadError: true so the page can render
 * a fallback.
 *
 * When the visitor is authed, also fetches their organizations via
 * GET /me/organizations so the claim flow can present an org picker
 * (BUG-004: the backend requires { org_id } on POST /services/{id}/claim).
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	// Independent of the service fetch — start it now so both run in
	// parallel. Best-effort and authed-only: failures resolve to [] so
	// the promise never rejects into the Promise.all below.
	const myOrgsPromise: Promise<{ id: string; name: string; slug: string }[]> = authed
		? apiList<{ id: string; name: string; slug: string }>(ctx, '/me/organizations')
				.then(({ items }) => items)
				.catch(() => [])
		: Promise.resolve([]);

	try {
		// Use apiGetEnvelope to access the sidecar `reviews` and
		// `review_aggregate` fields emitted by GET /services/:slug (BUG-014).
		const [envelope, myOrgs] = await Promise.all([
			apiGetEnvelope<ServiceListing>(ctx, `/services/${slug}`),
			myOrgsPromise
		]);
		const service = envelope.data;
		const reviews = (envelope as unknown as { reviews?: ServiceReview[] }).reviews ?? [];
		const reviewAggregate = (envelope as unknown as { review_aggregate?: ServiceReviewAggregate })
			.review_aggregate ?? { count: 0, avg: 0 };

		// "Recommended Services" card — same-type items ranked by the
		// related endpoint's content_type filter. Best-effort; the card
		// hides itself when the list is empty. NB: the endpoint reads
		// `page_size`, not `limit`.
		// §114: categories ride along so the hero badge can render the
		// admin-managed accent color/icon/label for the listing's category.
		const [recommendedRes, categoriesRes] = await Promise.allSettled([
			apiGet<ContentArticle[]>(
				ctx,
				`/content/${service.id}/related?content_type=service_listing&page_size=5`
			),
			apiGet<ServiceCategoryInfo[]>(ctx, '/services/categories')
		]);
		const recommendedContent: ContentArticle[] =
			recommendedRes.status === 'fulfilled' ? (recommendedRes.value ?? []) : [];
		const serviceCategories: ServiceCategoryInfo[] =
			categoriesRes.status === 'fulfilled' ? (categoriesRes.value ?? []) : [];

		return {
			service,
			authed,
			currentUserId,
			myOrgs,
			reviews,
			reviewAggregate,
			recommendedContent,
			serviceCategories,
			loadError: false
		};
	} catch (err) {
		// The org fetch is already settled-or-resolving; collect it so the
		// degraded payload keeps whatever it produced.
		const myOrgs = await myOrgsPromise;
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Service not found' });
		}
		return {
			service: null,
			authed,
			currentUserId,
			myOrgs,
			reviews: [] as ServiceReview[],
			reviewAggregate: { count: 0, avg: 0 },
			recommendedContent: [] as ContentArticle[],
			serviceCategories: [] as ServiceCategoryInfo[],
			loadError: true
		};
	}
};
