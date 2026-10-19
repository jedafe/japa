import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, apiPut, ApiError } from '$lib/server/api';
import { formatMoney } from '$lib/utils/currency';
import type {
	ColCity,
	ColCitiesListResponse,
	ColComparisonMatrix,
	ColCityData,
	ColCategoryData,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';

/**
 * Cost of Living Calculator at /tools/cost-of-living (Section 72, ADR-072).
 * SSR-fetches the city list so the selector renders without a client
 * round-trip. The comparison matrix is fetched client-side when the user
 * selects cities (shareable via ?city_ids= query param).
 *
 * Tool config (max_cities, stale_days) is read from the root layout's
 * siteSettings (col_calculator.* system_settings) so the UI never hardcodes
 * configurable values (Rule 3 / Rule 16).
 *
 * Ref: japaconnect-engineering/sections/72-cost-of-living.md
 */
export const load: PageServerLoad = async (event) => {
	const { features, siteSettings } = await event.parent();
	if (!features['col_calculator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Cost of Living Calculator is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Tool config from the root layout's public siteSettings (Rule 3 / Rule 16).
	// Public settings serialise ints as JSON strings — coerce leniently.
	const configuredMax = Number(siteSettings?.['col_calculator.max_cities']);
	const maxCities =
		Number.isInteger(configuredMax) && configuredMax > 0 ? configuredMax : 3;
	const configuredStale = Number(siteSettings?.['col_calculator.stale_days']);
	const staleDays =
		Number.isInteger(configuredStale) && configuredStale > 0 ? configuredStale : 180;

	// Pre-select from shareable URL ?city_ids=a,b,c&baseline_id=x and SSR the
	// matrix if present. baseline_id names the ratio reference city (§72
	// amendment: user-selectable baseline).
	const cityIdsParam = event.url.searchParams.get('city_ids');
	const baselineParam = event.url.searchParams.get('baseline_id');
	// Dedupe + keep only UUID-shaped values so crafted URLs
	// (?city_ids=a,a or ?city_ids=not-a-uuid) can't produce duplicate keyed
	// rows client-side or trip the compare endpoint's 400s.
	const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
	const preselected = cityIdsParam
		? [...new Set(cityIdsParam.split(',').filter((id) => UUID_RE.test(id.trim())))]
		: [];
	const preselectedBaseline =
		baselineParam && UUID_RE.test(baselineParam.trim()) ? baselineParam.trim() : null;

	// The matrix needs only the URL params — it rides the same wave as the
	// city list.
	const matrixPromise = (async () => {
		if (preselected.length === 0) return null;
		try {
			const params = new URLSearchParams({ city_ids: preselected.join(',') });
			if (preselectedBaseline) params.set('baseline_id', preselectedBaseline);
			return await apiGet<ColComparisonMatrix>(
				ctx,
				`/calculator/cost-of-living/compare?${params.toString()}`
			);
		} catch {
			return null;
		}
	})();

	let cities: ColCity[] = [];
	let disabled = false;
	try {
		const res = await apiGet<ColCitiesListResponse>(ctx, '/calculator/cost-of-living/cities?page=1&limit=200');
		cities = res.cities ?? [];
	} catch (e) {
		if (e instanceof ApiError && (e.code === 'col_calculator_disabled' || e.status === 404)) {
			disabled = true;
		}
	}

	const matrix = await matrixPromise;

	return { cities, disabled, authed, preselected, preselectedBaseline, maxCities, staleDays, matrix };
};

/**
 * Export the selected cost-of-living cities to a private JCE quick comparison.
 * Items are resolved through the `col_city` item_ref resolver, and the matrix
 * values (overall index + each cost category) are seeded as dimension values.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const cityIds = form
			.getAll('city_id')
			.filter((v): v is string => typeof v === 'string');
		const baselineId = String(form.get('baseline_id') ?? '').trim();

		if (cityIds.length < 2) {
			return fail(400, {
				error: 'invalid_selection',
				message: 'Select a baseline and at least one destination city'
			});
		}

		const ctx = apiContext(event);

		let matrix: ColComparisonMatrix;
		try {
			const params = new URLSearchParams({ city_ids: cityIds.join(',') });
			if (baselineId) params.set('baseline_id', baselineId);
			matrix = await apiGet<ColComparisonMatrix>(
				ctx,
				`/calculator/cost-of-living/compare?${params.toString()}`
			);
		} catch (e) {
			if (e instanceof ApiError) {
				// Pass 4xx validation errors (invalid_city_id, invalid_baseline_id,
				// too_many_cities) through with their status instead of
				// collapsing them into a generic 500.
				const status = e.status >= 400 && e.status < 500 ? e.status : 500;
				return fail(status, { error: e.code, message: e.message });
			}
			return fail(500, {
				error: 'fetch_failed',
				message: 'Could not load cost-of-living comparison'
			});
		}

		const cityById = new Map<string, ColCityData>(matrix.cities.map((c) => [c.id, c]));
		const orderedCities = cityIds
			.map((id) => cityById.get(id))
			.filter((c): c is ColCityData => !!c);

		if (orderedCities.length !== cityIds.length) {
			return fail(500, {
				error: 'city_not_found',
				message: 'One or more selected cities could not be resolved'
			});
		}

		const items = orderedCities.map((city, i) => ({
			title: city.title,
			description: city.geo_space_name || '',
			position: i,
			item_ref: { type: 'col_city', id: city.id }
		}));

		const dimensionLabels = [
			'Overall Cost Index',
			...matrix.categories.map((c: ColCategoryData) => c.label)
		];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Cost of living comparison',
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

		const itemById = new Map<string, ComparisonItem>();
		const itemByTitle = new Map<string, ComparisonItem>();
		for (const item of jce.items) {
			const ref = item.item_ref as { id?: string } | null | undefined;
			if (ref?.id) {
				itemById.set(ref.id, item);
			}
			itemByTitle.set(item.title, item);
		}

		function findItem(city: ColCityData): ComparisonItem | undefined {
			return itemById.get(city.id) ?? itemByTitle.get(city.title);
		}

		const dimByLabel = new Map<string, ComparisonDimension>(
			jce.dimensions.map((d) => [d.label, d])
		);

		const valueGetters: Record<string, (city: ColCityData) => string | undefined> = {
			'Overall Cost Index': (city) => {
				const v = matrix.overall_index[city.id];
				return v != null ? `${v.toFixed(2)}×` : undefined;
			}
		};
		for (const cat of matrix.categories) {
			valueGetters[cat.label] = (city) => {
				const v = cat.values[city.id];
				// Keep decimals — values like a $3.50 meal must not round to 4.
				return v != null ? formatMoney(v, matrix.display_currency || 'USD') : undefined;
			};
		}

		for (const [label, getter] of Object.entries(valueGetters)) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orderedCities
				.map((city) => {
					const item = findItem(city);
					if (!item) return null;
					const raw = getter(city);
					return { item_id: item.id, value: raw == null ? '' : raw };
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
