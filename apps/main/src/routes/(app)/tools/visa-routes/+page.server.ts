// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList, apiPost, apiPut, ApiError } from '#lib/server/api.js';
import type { VisaRouteWithAi, JCEComparison, ComparisonCreated, ComparisonItem, GeoCountry } from '#lib/types/index.js';

/**
 * Visa Route Comparison — §90, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the routes for that destination are fetched server-side
 * so the page renders with content on first paint (SSR).
 *
 * The "Open in JCE comparison" action creates a private quick comparison from
 * selected visa routes, seeds the matrix with tool-native values, and redirects
 * to the canonical /compare/{slug} preview.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /visa-routes?dest={space_id} — visa routes for a destination.
 *      GET /visa-routes/compare?ids=... — tool-specific detail fetch.
 *      POST /jce/comparisons — JCE quick comparison creation.
 *      PUT /jce/comparisons/dimensions/{id}/values — seed cell values.
 *
 * Route: /tools/visa-routes
 * Ref: §90 (Visa Route Comparison), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['visa_routes.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Visa Route Comparison is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// If ?dest= is present, fetch routes for that destination so the page
	// renders with content on first paint (SSR). The backend validates the
	// dest itself, so the routes call rides the same wave as the country
	// list — a bogus ?dest= degrades to an empty list either way.
	const destParam = event.url.searchParams.get('dest');
	const routesPromise = destParam
		? apiList<VisaRouteWithAi>(ctx, `/visa-routes?dest=${encodeURIComponent(destParam)}`)
				.then((r) => r.items)
				.catch(() => [] as VisaRouteWithAi[])
		: Promise.resolve([] as VisaRouteWithAi[]);

	// ?ids=a,b,c — shareable comparison selection (§90). When a link
	// carries ids but no dest, hydrate the referenced routes through the
	// compare endpoint so the recipient's selection survives the trip
	// (otherwise an empty route list silently drops it client-side).
	const initialRouteIds = (event.url.searchParams.get('ids') ?? '')
		.split(',')
		.map((s) => s.trim())
		.filter(Boolean)
		.slice(0, 4);

	const sharePromise = !destParam && initialRouteIds.length > 0
		? apiGetEnvelope<VisaRouteWithAi[]>(ctx, `/visa-routes/compare?ids=${initialRouteIds.map(encodeURIComponent).join(',')}`)
				.then((r) => ({
					items: r.data ?? [],
					missing: ((r as { missing_ids?: string[] }).missing_ids ?? []) as string[]
				}))
				.catch((e) => ({
					// A 404 means every requested id failed to resolve —
					// report them all as missing so the recipient sees the
					// "no longer available" notice, not a bare empty state.
					items: [] as VisaRouteWithAi[],
					missing: e instanceof ApiError && e.status === 404 ? [...initialRouteIds] : []
				}))
		: Promise.resolve({ items: [] as VisaRouteWithAi[], missing: [] as string[] });

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or visa_routes_disabled code means the tool is turned off.
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'visa_routes_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	const share = await sharePromise;
	const routes = destParam ? await routesPromise : share.items;
	let initialDestId = destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	// ?dest= also resolves geo_codes/slugs server-side ("CA" works in the
	// API but isn't a picker UUID). Match the raw value against the
	// picker's geo codes first — this is what selects the picker when the
	// destination has no routes yet and nothing can be derived below.
	if (!initialDestId && destParam) {
		const byCode = destinations.find(
			(d) => d.geo_code.toUpperCase() === destParam.toUpperCase()
		);
		if (byCode) initialDestId = byCode.id;
	}

	// When the loaded routes all agree on a destination space (slug dests,
	// or bare ?ids= shares that resolved to one country), reflect it in
	// the picker instead of showing the empty state on a page that has
	// data.
	if (!initialDestId && routes.length > 0) {
		const d0 = routes[0].destination_space_id;
		if (
			d0 &&
			routes.every((r) => r.destination_space_id === d0) &&
			destinations.some((d) => d.id === d0)
		) {
			initialDestId = d0;
		}
	}

	return {
		destinations,
		routes,
		initialDestId,
		initialRouteIds,
		initialMissingIds: share.missing,
		disabled,
		authed
	};
};

/**
 * Build a private quick JCE comparison from 2-4 selected visa routes.
 * The matrix is pre-seeded with tool-native fields (fee, path to PR, renewal
 * terms, restrictions, official source) so the /compare/{slug} preview is
 * immediately useful.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const routeIds = form
			.getAll('route_id')
			.filter((v): v is string => typeof v === 'string');
		const destId = form.get('dest_id');

		if (routeIds.length < 2 || routeIds.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 visa routes' });
		}

		const ctx = apiContext(event);

		// Fetch the selected routes in the order the user picked them.
		// dest_id is optional — a shared ?ids= link can span destinations,
		// in which case no destination filter is applied.
		const destParam =
			typeof destId === 'string' && destId ? `&dest=${encodeURIComponent(destId)}` : '';
		let routes: VisaRouteWithAi[];
		try {
			routes = await apiGet<VisaRouteWithAi[]>(
				ctx,
				`/visa-routes/compare?ids=${routeIds.map(encodeURIComponent).join(',')}${destParam}`
			);
		} catch (e) {
			// Forward the API's client errors (bad id → 400) instead of
			// flattening every failure into a 500.
			const status = e instanceof ApiError && e.status >= 400 && e.status < 600 ? e.status : 500;
			const message = e instanceof ApiError ? e.message : 'Could not load visa routes';
			return fail(status, { error: 'fetch_failed', message });
		}

		const routeById = new Map(routes.map((r) => [r.id, r]));
		const orderedRoutes = routeIds
			.map((id) => routeById.get(id))
			.filter((r): r is VisaRouteWithAi => !!r);

		if (orderedRoutes.length !== routeIds.length) {
			// A selected id stopped resolving (unpublished/deactivated
			// since the link was made) — that's a client-visible selection
			// problem, not a server fault.
			return fail(400, { error: 'route_not_found', message: 'One or more selected routes could not be resolved' });
		}

		const items = orderedRoutes.map((r, i) => ({
			title: r.name,
			description: r.eligibility_summary ?? '',
			position: i,
			item_ref: { type: 'visa_route', id: r.id }
		}));

		const dimensionLabels = ['Typical fee', 'Path to PR', 'Renewal terms', 'Restrictions', 'Official source'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Visa route comparison',
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

		// Match created items back to routes by item_ref.id — route names
		// aren't unique across destinations, so title matching can collide.
		// Falls back to title for items whose item_ref didn't round-trip.
		const itemByRefId = new Map<string, ComparisonItem>();
		const itemByTitle = new Map<string, ComparisonItem>();
		for (const item of jce.items) {
			const refId = typeof item.item_ref?.id === 'string' ? item.item_ref.id : null;
			if (refId) itemByRefId.set(refId, item);
			itemByTitle.set(item.title, item);
		}
		const dimByLabel = new Map(jce.dimensions.map((d) => [d.label, d]));

		const fieldMap: Record<string, keyof VisaRouteWithAi> = {
			'Typical fee': 'typical_fee_usd',
			'Path to PR': 'path_to_pr',
			'Renewal terms': 'renewal_terms',
			'Restrictions': 'restrictions',
			'Official source': 'gov_source_url'
		};

		for (const [label, field] of Object.entries(fieldMap)) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orderedRoutes
				.map((route) => {
					const item = itemByRefId.get(route.id) ?? itemByTitle.get(route.name);
					if (!item) return null;
					const raw = route[field];
					let value = '';
					if (raw == null) {
						value = '';
					} else if (typeof raw === 'number') {
						value = new Intl.NumberFormat('en-US', {
							style: 'currency',
							currency: 'USD',
							maximumFractionDigits: 0
						}).format(raw);
					} else {
						value = String(raw);
					}
					return { item_id: item.id, value };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				// Seed failures are non-fatal; the user can still see the
				// created comparison and fill values in the composer.
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
