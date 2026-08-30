import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import {
	normalizeLegacyBlocks,
	resolveGeoCopy,
	resolveString,
	getGeoTokens,
	type Block,
} from '@japaconnect/landing-blocks';
import type { GeoContext, GeoCountry, PageBuilderPage } from '$lib/types';

/**
 * Page Builder landing page at /pages/{slug} — public, SSR. Renders the
 * published page via the shared LandingRenderer component. Geo context and
 * server-resolved template literals are applied server-side before the page
 * reaches the client.
 *
 * Ref: japaconnect-engineering/sections/27-page-builder.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Geo resolution priority: ?geo= query param → GeoIP → 'global'.
	let geoCountry =
		event.url.searchParams.get('geo') ??
		event.locals.user?.content_country_preference ??
		null;
	let geoCountryName: string | undefined;
	if (!geoCountry) {
		try {
			const geo = await apiGet<GeoContext>(ctx, '/geo/me');
			if (geo?.country_code) {
				geoCountry = geo.country_code;
				geoCountryName = geo.country_name || undefined;
			}
			// /geo/me returns country_code/country_name; when the country name is
			// missing or generic, we look it up in the country catalog below.
		} catch {
			// GeoIP not available (no database, localhost, etc.) — fall back.
		}
	}

	// When geo was set via ?geo= param (not GeoIP), resolve the country name
	// from the backend so any of the seeded countries gets its real name.
	if (geoCountry && geoCountry !== 'global' && !geoCountryName) {
		try {
			const countries = await apiGet<GeoCountry[]>(ctx, '/geo/countries');
			const match = countries.find((c) => c.geo_code === geoCountry!.toUpperCase());
			if (match) geoCountryName = match.name;
		} catch {
			// Countries endpoint unavailable — getGeoTokens will use the
			// country code as a fallback name.
		}
	}

	if (!geoCountry) geoCountry = 'global';

	try {
		const page = await apiGet<PageBuilderPage>(ctx, `/pages/${slug}`);
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);
		const literals = page.literals ?? {};

		if (page.title) {
			page.title = resolveString(page.title, tokens, literals);
		}
		if (page.description) {
			page.description = resolveString(page.description, tokens, literals);
		}
		if (page.meta) {
			if (typeof page.meta.title === 'string') {
				page.meta.title = resolveString(page.meta.title, tokens, literals);
			}
			if (typeof page.meta.description === 'string') {
				page.meta.description = resolveString(page.meta.description, tokens, literals);
			}
		}

		const blocks = normalizeLegacyBlocks(page.blocks) as Block[];
		page.blocks = resolveGeoCopy(blocks, {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
			literals,
		}) as unknown as typeof page.blocks;

		return { page, contentCountry: geoCountry };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Page not found' });
		}
		throw err;
	}
};
