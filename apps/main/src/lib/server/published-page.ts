import { error } from '@sveltejs/kit';
import type { ServerLoadEvent } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '#lib/server/api.js';
import {
	normalizeLegacyBlocks,
	resolveGeoCopy,
	resolveString,
	getGeoTokens,
	containsGuestBlocks,
	type Block,
} from '@japaconnect/landing-blocks';
import { buildGuestHome, emptyGuestHome } from '#lib/server/guest-home.js';
import type { GeoContext, GeoCountry, PageBuilderPage } from '#lib/types/index.js';

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
/**
 * Path for a geo-cacheable published-page fetch. `geo` carries the resolved
 * destination country ('all' for global) so the backend resolves the same
 * literals a GeoIP lookup would; `_src` carries the visitor's detected IP
 * country purely for the cache key (the backend ignores it) — Source_Country
 * literals resolve from the forwarded IP either way. The api layer only
 * serves these entries to anonymous callers (auth/journey context can't be
 * captured in the key).
 */
export function publishedPagePath(slug: string, geoCountry: string, srcGeo: string): string {
	const geo = geoCountry === 'global' ? 'all' : geoCountry.toLowerCase();
	return `/pages/${slug}?geo=${encodeURIComponent(geo)}&_src=${encodeURIComponent(srcGeo.toLowerCase())}`;
}

export async function resolveGeoContext(event: ServerLoadEvent): Promise<{
	geoCountry: string;
	geoCountryName: string | undefined;
	srcGeo: string;
}> {
	const ctx = apiContext(event);
	// ?geo= mirrors the backend's explicitGeoParam contract: 'all' is the
	// explicit-global override and a 2-letter code is a real override —
	// anything else (empty, malformed like ?geo=1A) is treated as absent so
	// resolution falls through to the user preference rather than rendering
	// garbage tokens (e.g. "1A") or skipping the preference.
	const geoParamRaw = event.url.searchParams.get('geo')?.trim();
	const geoParam =
		geoParamRaw && (/^[a-z]{2}$/i.test(geoParamRaw) || geoParamRaw.toLowerCase() === 'all')
			? geoParamRaw
			: null;
	let geoCountry = geoParam ?? event.locals.user?.content_country_preference ?? null;
	let geoCountryName: string | undefined;

	// ?geo=all is the documented explicit-global override (ADR-064 §2) —
	// normalize it to the 'global' sentinel so getGeoTokens doesn't render
	// "all" as a literal country name.
	if (geoCountry?.toLowerCase() === 'all') geoCountry = 'global';

	// The countries catalog doesn't depend on the resolved country — fire it
	// alongside GeoIP so a needed name lookup never waits a second round-trip.
	// It's a 5m-cached public catalog, so the speculative call is usually free.
	const countriesPromise = apiGet<GeoCountry[]>(ctx, '/geo/countries').catch(() => null);

	// GeoIP always resolves — it is the visitor's source country for the
	// geo-keyed page cache even when a ?geo= param or preference already
	// fixed the destination. IP-cached in the api layer, so repeat visitors
	// pay ~nothing for this hop.
	const geoMePromise = apiGet<GeoContext>(ctx, '/geo/me').catch(() => null);

	if (!geoCountry) {
		const geo = await geoMePromise;
		if (geo?.country_code) {
			geoCountry = geo.country_code;
			geoCountryName = geo.country_name || undefined;
		}
		// /geo/me returns country_code/country_name; when the country name is
		// missing or generic, we look it up in the country catalog below.
	}

	// When geo was set via ?geo= param (not GeoIP), resolve the country name
	// from the backend so any of the seeded countries gets its real name.
	if (geoCountry && geoCountry !== 'global' && !geoCountryName) {
		const countries = await countriesPromise;
		const match = countries?.find((c) => c.geo_code === geoCountry!.toUpperCase());
		if (match) geoCountryName = match.name;
	}

	const srcGeo = (await geoMePromise)?.country_code ?? '';

	return { geoCountry: geoCountry || 'global', geoCountryName, srcGeo };
}

export async function loadPublishedPage(event: ServerLoadEvent) {
	const ctx = apiContext(event);
	const slug = event.params.slug!;

	// Geo resolves first so the page fetch can carry it explicitly — that
	// makes the payload cacheable per (destination, source) via
	// publishedPagePath instead of being a per-IP uncacheable response.
	// /geo/me is IP-cached, so this hop is ~free on repeat visits.
	const { geoCountry, geoCountryName, srcGeo } = await resolveGeoContext(event);

	try {
		const envelope = await apiGetEnvelope<PageBuilderPage>(
			ctx,
			publishedPagePath(slug, geoCountry, srcGeo)
		);
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
