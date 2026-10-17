import { error } from '@sveltejs/kit';
import type { ServerLoadEvent } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import {
	normalizeLegacyBlocks,
	resolveGeoCopy,
	resolveString,
	getGeoTokens,
	containsGuestBlocks,
	type Block,
} from '@japaconnect/landing-blocks';
import { buildGuestHome, emptyGuestHome } from '$lib/server/guest-home';
import type { GeoContext, GeoCountry, PageBuilderPage } from '$lib/types';

/**
 * Shared server loader for published Page Builder pages. Used by both
 * /pages/[slug] (legacy path, kept for compatibility) and the root-level
 * /[slug] route so published pages render at clean URLs (/about-us, /terms).
 *
 * Geo context and server-resolved template literals are applied server-side
 * before the page reaches the client.
 *
 * Ref: japaconnect-engineering/sections/27-page-builder.md
 */
/**
 * Resolve the visitor's geo context for Page Builder rendering.
 * Priority: ?geo= query param → user preference → GeoIP → 'global'.
 * Shared by loadPublishedPage and other routes that render builder blocks
 * (e.g. the /wiki index hero).
 */
export async function resolveGeoContext(event: ServerLoadEvent): Promise<{
	geoCountry: string;
	geoCountryName: string | undefined;
}> {
	const ctx = apiContext(event);
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

	return { geoCountry: geoCountry || 'global', geoCountryName };
}

export async function loadPublishedPage(event: ServerLoadEvent) {
	const ctx = apiContext(event);
	const slug = event.params.slug!;

	const { geoCountry, geoCountryName } = await resolveGeoContext(event);

	try {
		const envelope = await apiGetEnvelope<PageBuilderPage>(ctx, `/pages/${slug}`);
		const page = envelope.data;
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);
		const literals = envelope.literals ?? {};

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

		// Guest-home context data for any guest_* blocks on the page — they
		// render context data (stats, categories, stories, …) even without a
		// guest_home wrapper on the page.
		const home = containsGuestBlocks(blocks) ? await buildGuestHome(ctx, event) : emptyGuestHome;

		return { page, contentCountry: geoCountry, home };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Page not found' });
		}
		throw err;
	}
}
