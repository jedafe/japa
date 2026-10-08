// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { ServiceListing, ServiceCategoryInfo, GeoCountry, Space } from '$lib/types';

/**
 * Service Directory list at /services — geo-indexed directory of
 * diaspora-relevant businesses and service providers. SSR-fetches
 * GET /services?dest=&city=&category=&origin=&verified=&sort=&q=&kind=&page=&limit=,
 * passing through all query params so the backend filters and sorts
 * server-side.
 *
 * `dest`/`city` accept a space UUID or a space slug; slugs are resolved to
 * the space id (and display name) via GET /spaces/{slug} before the list
 * call so the URL stays human-readable.
 */
const DEFAULT_PAGE_SIZE = 20;
const MAX_PAGE_SIZE = 100; // backend caps ?limit= at 100
const VALID_SORTS = new Set(['latest', 'top']);
const VALID_KINDS = new Set(['service', 'package', 'all']);
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function parsePositiveInt(raw: string | null, fallback: number, max: number): number {
	const n = Number(raw);
	return Number.isInteger(n) && n > 0 ? Math.min(n, max) : fallback;
}

/**
 * Resolve a ?dest=/?city= param to a space id + display name. GET
 * /spaces/{slug} accepts either a slug or a space UUID, so both forms
 * resolve to a name here — the backend falls back to id lookup when the
 * param parses as a UUID. Returns null when the ref cannot be resolved.
 */
async function resolveSpaceRef(
	ctx: ReturnType<typeof apiContext>,
	raw: string
): Promise<{ id: string; name: string } | null> {
	if (!raw) return null;
	try {
		const res = await apiGet<{ space?: Space }>(ctx, `/spaces/${encodeURIComponent(raw)}`);
		return res?.space ? { id: res.space.id, name: res.space.name } : null;
	} catch {
		// UUIDs may refer to spaces the endpoint can't serve — keep the
		// filter working on the id even when the label can't be resolved.
		return UUID_RE.test(raw) ? { id: raw, name: '' } : null;
	}
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_directory.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service Directory is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const packagesEnabled = !!features['service_packages.enabled'];

	const get = (k: string) => event.url.searchParams.get(k)?.trim() ?? '';
	const q = get('q');
	const dest = get('dest');
	const city = get('city');
	const category = get('category');
	const origin = get('origin');
	const verified = get('verified');
	const sortRaw = get('sort');
	const sort = VALID_SORTS.has(sortRaw) ? sortRaw : 'latest';
	const kindRaw = get('kind');
	// §111: kind tabs only exist while service_packages.enabled is on —
	// kind=package against a disabled flag is a backend 404.
	const kind = packagesEnabled && VALID_KINDS.has(kindRaw) ? kindRaw : '';
	// §111 package filters — price_max is minor units on the API contract;
	// date_from is an ISO availability-start filter. Both are pass-through
	// and only meaningful when packages are enabled.
	const priceMaxRaw = get('price_max');
	const priceMax = packagesEnabled && /^\d+$/.test(priceMaxRaw) ? priceMaxRaw : '';
	const dateFromRaw = get('date_from');
	const dateFrom = packagesEnabled && /^\d{4}-\d{2}-\d{2}$/.test(dateFromRaw) ? dateFromRaw : '';
	const page = parsePositiveInt(event.url.searchParams.get('page'), 1, Number.MAX_SAFE_INTEGER);
	const limit = parsePositiveInt(event.url.searchParams.get('limit'), DEFAULT_PAGE_SIZE, MAX_PAGE_SIZE);

	// Resolve slug-form geo params to space ids before calling the API.
	const destRef = await resolveSpaceRef(ctx, dest);
	const cityRef = await resolveSpaceRef(ctx, city);
	const destId = destRef?.id ?? '';
	const cityId = cityRef?.id ?? '';

	// filters.dest carries the resolved space id so the country Autocomplete
	// chip matches its item ids; filters.city echoes the raw URL param (slug
	// preferred — the city picker writes slugs so URLs stay readable) and
	// cityLabel feeds its chip label.
	const filters = {
		q,
		dest: destId,
		destLabel: destRef?.name ?? '',
		city: cityId ? city : '',
		cityLabel: cityRef?.name ?? '',
		category,
		origin,
		verified,
		sort,
		kind,
		// price_max echoes as minor units (contract); the UI input collects
		// major units, so seed it pre-divided.
		priceMaxMajor: priceMax ? String(Number(priceMax) / 100) : '',
		date_from: dateFrom,
		page,
		limit
	};

	const params = new URLSearchParams();
	params.set('limit', String(limit));
	if (page > 1) params.set('page', String(page));
	if (q) params.set('q', q);
	if (destId) params.set('dest', destId);
	if (cityId) params.set('city', cityId);
	if (category) params.set('category', category);
	if (origin) params.set('origin', origin);
	if (verified === 'true' || verified === '1') params.set('verified', 'true');
	if (sort !== 'latest') params.set('sort', sort);
	if (kind) params.set('kind', kind);
	if (priceMax) params.set('price_max', priceMax);
	if (dateFrom) params.set('date_from', dateFrom);

	// ?submitted=1 — set by /services/new after a successful create, since
	// the new listing enters `in_review` and its detail URL is gated to
	// published rows (§111 review gate).
	const submitted = event.url.searchParams.get('submitted') === '1';

	const [countriesRes, categoriesRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		// §114: the endpoint returns the full taxonomy Option row
		// (icon/accent_color/description/applies_to) — keep the whole object
		// so the marketplace UI can render accent chips + icons.
		apiGet<ServiceCategoryInfo[]>(ctx, '/services/categories')
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	const serviceCategories: ServiceCategoryInfo[] =
		categoriesRes.status === 'fulfilled' ? (categoriesRes.value ?? []) : [];

	try {
		const { items, meta } = await apiList<ServiceListing>(ctx, `/services?${params.toString()}`);
		return {
			services: items,
			meta,
			authed,
			packagesEnabled,
			filters,
			countries,
			serviceCategories,
			submitted,
			loadError: false
		};
	} catch {
		return {
			services: [] as ServiceListing[],
			meta: undefined,
			authed,
			packagesEnabled,
			filters,
			countries,
			serviceCategories,
			submitted,
			loadError: true
		};
	}
};
