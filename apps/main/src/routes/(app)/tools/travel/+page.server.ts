// Ref: japaconnect-engineering/sections/67-travel-search.md (ADR-085)
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, apiPut, ApiError } from '$lib/server/api';
import type { JCEComparison, ComparisonCreated, ComparisonItem, ComparisonDimension } from '$lib/types';

interface TravelProvider {
	id: string;
	name: string;
	slug: string;
	provider_type: string;
	is_active: boolean;
	created_at: string;
}

/**
 * Travel Search at /tools/travel (Section 67, ADR-085).
 * The backend gate (travel.enabled) is enforced inside the handler; the
 * page renders the search form SSR and fetches results client-side.
 *
 * The "Compare in JCE" action creates a private quick comparison from
 * 2-4 selected travel providers (the resolver-backed entity for this surface)
 * and redirects to /compare/{slug}.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['travel.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Travel Search is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let providers: TravelProvider[] = [];
	try {
		providers = await apiGet<TravelProvider[]>(ctx, '/travel/providers');
	} catch (e) {
		// Non-fatal: the search UI still works if the provider list fails.
		console.error('failed to load travel providers:', e);
	}

	return { authed, providers };
};

/**
 * Build a private quick JCE comparison from 2-4 selected travel providers.
 * The matrix is pre-seeded with provider-native fields.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const providerIds = form
			.getAll('provider_id')
			.filter((v): v is string => typeof v === 'string');

		if (providerIds.length < 2 || providerIds.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 providers' });
		}

		const ctx = apiContext(event);

		let providers: TravelProvider[];
		try {
			providers = await apiGet<TravelProvider[]>(ctx, '/travel/providers');
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load providers';
			return fail(500, { error: 'fetch_failed', message });
		}

		const providerById = new Map<string, TravelProvider>(providers.map((p) => [p.id, p]));
		const selected = providerIds
			.map((id) => providerById.get(id))
			.filter((p): p is TravelProvider => !!p);

		if (selected.length !== providerIds.length) {
			return fail(400, { error: 'invalid_selection', message: 'One or more selected providers are not valid' });
		}

		const items = selected.map((p, i) => ({
			title: p.name,
			description: '',
			position: i,
			item_ref: { type: 'travel', id: p.id }
		}));

		const dimensionLabels = ['Provider Type', 'Active Since', 'Slug'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Travel provider comparison',
				depth: 'quick',
				body: '',
				items,
				dimensions: dimensionLabels
			});
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not create comparison';
			return fail(500, { error: 'create_failed', message });
		}

		let jce: JCEComparison;
		try {
			jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${created.id}`);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load created comparison';
			return fail(500, { error: 'detail_failed', message });
		}

		const itemByPosition = new Map<number, ComparisonItem>();
		for (const [i, item] of jce.items.entries()) {
			itemByPosition.set(i, item);
		}
		const dimByLabel = new Map<string, ComparisonDimension>(
			jce.dimensions.map((d) => [d.label, d])
		);

		function getValue(p: TravelProvider, label: string): string {
			switch (label) {
				case 'Provider Type':
					return p.provider_type;
				case 'Active Since':
					return p.created_at ? new Date(p.created_at).toLocaleDateString() : '';
				case 'Slug':
					return p.slug;
				default:
					return '';
			}
		}

		for (const label of dimensionLabels) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = selected
				.map((p, i) => {
					const item = itemByPosition.get(i);
					if (!item) return null;
					return { item_id: item.id, value: getValue(p, label) };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
